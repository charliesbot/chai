// Package doctor inspects Chai-managed skill destinations without changing them.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/skill"
	"github.com/charliesbot/chai/internal/skillsource"
)

type findingKind int

const (
	unmanagedSkill findingKind = iota
	blockingDestination
)

type finding struct {
	kind     findingKind
	name     string
	platform string
	path     string
}

// Run checks the configured platforms using the current user's home directory.
func Run(ctx context.Context, cfg *config.Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	return RunWithHome(ctx, cfg, home)
}

// RunWithHome checks configured skill destinations without creating, modifying,
// or deleting files. It returns an error only when sync is blocked or inspection
// cannot be completed.
func RunWithHome(ctx context.Context, cfg *config.Config, home string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("doctor interrupted: %w", err)
	}
	sources, err := skillsource.Resolve(cfg, home)
	if err != nil {
		return err
	}
	hashDB, err := hash.Load(home)
	if err != nil {
		return err
	}

	findings, err := inspect(home, platform.ForNames(cfg.Platforms), sources, hashDB)
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		fmt.Println("doctor: no unmanaged skill destinations found")
		return nil
	}

	blocked := false
	for _, finding := range findings {
		printFinding(finding)
		blocked = blocked || finding.kind == blockingDestination
	}
	if blocked {
		return errors.New("unmanaged skill destinations block sync; move or remove them, then retry")
	}
	return nil
}

func inspect(home string, platforms []platform.Platform, sources []skill.Source, hashDB hash.DB) ([]finding, error) {
	expected := make(map[string]string, len(sources))
	for _, source := range sources {
		expected[skill.DirectoryName(source.Name)] = source.Name
	}
	expectedDirs := make([]string, 0, len(expected))
	for directoryName := range expected {
		expectedDirs = append(expectedDirs, directoryName)
	}
	sort.Strings(expectedDirs)

	seenDirs := make(map[string]bool, len(platforms))
	seenDestinations := make(map[string]bool, len(platforms)*len(sources))
	var findings []finding
	for _, target := range platforms {
		dir := filepath.Join(home, target.SkillsDir)

		// Match sync's collision semantics for every configured destination.
		// This must run independently of ReadDir: a file or symlink at a
		// configured path blocks sync even though it is not an unmanaged extra.
		for _, directoryName := range expectedDirs {
			name := expected[directoryName]
			path := filepath.Join(dir, directoryName)
			if seenDestinations[path] {
				continue
			}
			seenDestinations[path] = true
			_, err := os.Stat(path)
			if err == nil {
				if _, managed := hashDB[path]; !managed {
					findings = append(findings, finding{kind: blockingDestination, name: name, platform: target.Name, path: path})
				}
				continue
			}
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("checking skill destination %s: %w", path, err)
			}
		}

		if seenDirs[dir] {
			continue
		}
		seenDirs[dir] = true
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading skill destination %s: %w", dir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if _, configured := expected[entry.Name()]; configured {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if _, managed := hashDB[path]; managed {
				continue
			}
			findings = append(findings, finding{kind: unmanagedSkill, name: entry.Name(), platform: target.Name, path: path})
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].platform != findings[j].platform {
			return findings[i].platform < findings[j].platform
		}
		return findings[i].path < findings[j].path
	})
	return findings, nil
}
func printFinding(f finding) {
	if f.kind == blockingDestination {
		fmt.Printf("ERROR Unmanaged destination blocks configured skill: %s\n", f.name)
		fmt.Printf("      Platform: %s\n      Path: %s\n", f.platform, f.path)
		fmt.Println("      Not tracked by Chai; sync will not overwrite it.")
		fmt.Println("      Move or remove it, then run chai sync.")
		return
	}
	fmt.Printf("INFO  Unmanaged skill: %s\n", f.name)
	fmt.Printf("      Platform: %s\n      Path: %s\n", f.platform, f.path)
	fmt.Println("      Not tracked by Chai; left untouched.")
	fmt.Println("      No action required unless you want Chai to manage it.")
}
