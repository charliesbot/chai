package sync

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/resolve"
	"github.com/charliesbot/chai/internal/ui"
)

func syncInstructions(
	ctx context.Context,
	cfg *config.Config,
	home string,
	platforms []platform.Platform,
	opts Options,
	hashDB hash.DB,
) (error, error) {
	instructionPlatforms := platformsWithInstructions(platforms)
	if len(instructionPlatforms) == 0 {
		return nil, nil
	}
	if len(cfg.Instructions) == 0 {
		return nil, fmt.Errorf("no instructions path set in config")
	}

	status := newPlatformStatus(platforms)
	for _, p := range platforms {
		if p.InstructionsPath == "" {
			status.setNA(p.Name)
		}
	}

	srcPaths, content, err := loadInstructions(cfg.Instructions, home)
	if err != nil {
		return nil, err
	}
	if opts.DryRun {
		fmt.Println(ui.Label.Render("instructions"))
		for _, srcPath := range srcPaths {
			fmt.Printf("  %s %s\n", ui.Muted.Render("source:"), srcPath)
		}
	}

	// Group platforms by destination path so that platforms sharing an
	// instructions file only trigger one write and one dirty-detection prompt.
	destOrder := make([]string, 0, len(instructionPlatforms))
	destPlatforms := make(map[string][]platform.Platform, len(instructionPlatforms))
	for _, p := range instructionPlatforms {
		dest := filepath.Join(home, p.InstructionsPath)
		if _, ok := destPlatforms[dest]; !ok {
			destOrder = append(destOrder, dest)
		}
		destPlatforms[dest] = append(destPlatforms[dest], p)
	}
	instructionChanges := newItemChanges()
	skippedInstructionTargets := 0
	instructionName := filepath.Base(srcPaths[0])

	for _, dest := range destOrder {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("sync interrupted: %w", err)
		}

		sharers := destPlatforms[dest]
		if opts.DryRun {
			dryStatus := ui.Muted.Render("first sync")
			if _, ok := hashDB[dest]; ok {
				dirty, _ := hashDB.IsDirty(dest)
				if dirty {
					dryStatus = ui.Warning.Render("modified — will prompt")
				} else {
					dryStatus = ui.Muted.Render("unchanged")
				}
			}
			names := platformNames(sharers)
			fmt.Printf("  %s %s %s (%s)\n", ui.Arrow(), ui.Bold.Render(names), ui.Muted.Render(dest), dryStatus)
			continue
		}

		result, err := reconcileManagedFiles(
			filepath.Dir(dest),
			filepath.Ext(dest),
			[]managedDesired{{
				Name:       filepath.Base(dest),
				ChangeName: instructionName,
				Content:    content,
			}},
			hashDB,
			instructionReconciliationPolicy(opts),
		)
		if err != nil {
			return nil, fmt.Errorf("writing instructions to %s: %w", dest, err)
		}
		instructionChanges.merge(result.changes)
		if len(result.skipped) > 0 {
			skippedInstructionTargets++
			for _, p := range sharers {
				status.setFailed(p.Name)
			}
		}
	}

	if opts.DryRun {
		return nil, nil
	}

	summary := instructionChanges.summary()
	if skippedInstructionTargets > 0 {
		skipped := fmt.Sprintf("%d %s skipped", skippedInstructionTargets, pluralize("target", skippedInstructionTargets))
		if summary == "" {
			summary = skipped
		} else {
			summary += " · " + skipped
		}
	}
	fmt.Println(ui.ResultLine("instructions", summary, status.statuses()))
	for _, detail := range instructionChanges.details() {
		fmt.Printf("   %s\n", detail.render())
	}
	if skippedInstructionTargets > 0 {
		return fmt.Errorf("instruction sync incomplete: %d modified targets were preserved", skippedInstructionTargets), nil
	}
	return nil, nil
}

func loadInstructions(configuredPaths []string, home string) ([]string, []byte, error) {
	paths := make([]string, 0, len(configuredPaths))
	contents := make([][]byte, 0, len(configuredPaths))
	for _, configuredPath := range configuredPaths {
		srcPath, err := resolve.PathWithHome(configuredPath, home)
		if err != nil {
			return nil, nil, fmt.Errorf("resolving instructions path %q: %w", configuredPath, err)
		}
		content, err := os.ReadFile(srcPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil, fmt.Errorf("instructions file not found: %s", srcPath)
			}
			return nil, nil, fmt.Errorf("reading instructions %s: %w", srcPath, err)
		}
		paths = append(paths, srcPath)
		contents = append(contents, content)
	}
	if len(contents) == 1 {
		return paths, contents[0], nil
	}
	for i := range contents {
		contents[i] = bytes.Trim(contents[i], "\r\n")
	}
	return paths, bytes.Join(contents, []byte("\n\n")), nil
}

// platformStatus tracks success/failure per platform, preserving order for UI rendering.
type platformStatus struct {
	order         []string
	state         map[string]ui.PlatformState
	displayByName map[string]string
}

func newPlatformStatus(platforms []platform.Platform) platformStatus {
	ps := platformStatus{
		order:         make([]string, 0, len(platforms)),
		state:         make(map[string]ui.PlatformState, len(platforms)),
		displayByName: make(map[string]string, len(platforms)),
	}
	for _, p := range platforms {
		display := platformDisplayName(p)
		ps.displayByName[p.Name] = display
		if _, ok := ps.state[display]; !ok {
			ps.order = append(ps.order, display)
			ps.state[display] = ui.PlatformOK
		}
	}
	return ps
}

func platformDisplayName(p platform.Platform) string {
	if p.Key == "antigravity" {
		return "Antigravity"
	}
	return p.Name
}

func platformsWithInstructions(platforms []platform.Platform) []platform.Platform {
	out := make([]platform.Platform, 0, len(platforms))
	for _, p := range platforms {
		if p.InstructionsPath != "" {
			out = append(out, p)
		}
	}
	return out
}

func (ps platformStatus) setFailed(name string) {
	display := ps.displayName(name)
	if _, ok := ps.state[display]; ok {
		ps.state[display] = ui.PlatformFailed
	}
}

func (ps platformStatus) setNA(name string) {
	display := ps.displayName(name)
	if _, ok := ps.state[display]; ok {
		ps.state[display] = ui.PlatformNA
	}
}

func (ps platformStatus) displayName(name string) string {
	if display, ok := ps.displayByName[name]; ok {
		return display
	}
	return name
}

func (ps platformStatus) statuses() []ui.PlatformStatus {
	out := make([]ui.PlatformStatus, len(ps.order))
	for i, name := range ps.order {
		out[i] = ui.PlatformStatus{Name: name, State: ps.state[name]}
	}
	return out
}

// platformNames joins the names of the given platforms with " + " for display.
func platformNames(platforms []platform.Platform) string {
	names := make([]string, len(platforms))
	for i, p := range platforms {
		names[i] = p.Name
	}
	return strings.Join(names, " + ")
}
