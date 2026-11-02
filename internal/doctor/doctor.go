// Package doctor inspects Chai-managed skill destinations without changing them.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/skill"
	"github.com/charliesbot/chai/internal/skilldest"
	"github.com/charliesbot/chai/internal/skillsource"
)

type findingKind int

const (
	unmanagedSkill findingKind = iota
	blockingDestination
)

type finding struct {
	kind      findingKind
	name      string
	platforms []string
	path      string
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
	names := make([]string, len(sources))
	configured := make(map[string]bool, len(sources))
	for i, source := range sources {
		names[i] = source.Name
		configured[skill.DirectoryName(source.Name)] = true
	}

	collisions, err := skilldest.Collisions(home, platforms, names, hashDB)
	if err != nil {
		return nil, err
	}
	findings := make([]finding, 0, len(collisions))
	for _, collision := range collisions {
		findings = append(findings, finding{kind: blockingDestination, name: collision.Name, platforms: collision.Platforms, path: collision.Path})
	}

	// Platforms can share a skills directory; scan each directory once and
	// attribute its extras to every platform that uses it.
	var dirs []string
	platformsByDir := make(map[string][]string, len(platforms))
	for _, target := range platforms {
		dir := filepath.Join(home, target.SkillsDir)
		if _, seen := platformsByDir[dir]; !seen {
			dirs = append(dirs, dir)
		}
		platformsByDir[dir] = append(platformsByDir[dir], target.Name)
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading skill destination %s: %w", dir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || configured[entry.Name()] {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if !skilldest.UnmanagedSkill(path, hashDB) {
				continue
			}
			findings = append(findings, finding{kind: unmanagedSkill, name: entry.Name(), platforms: platformsByDir[dir], path: path})
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].platforms[0] != findings[j].platforms[0] {
			return findings[i].platforms[0] < findings[j].platforms[0]
		}
		return findings[i].path < findings[j].path
	})
	return findings, nil
}

func printFinding(f finding) {
	if f.kind == blockingDestination {
		fmt.Printf("ERROR Unmanaged destination blocks configured skill: %s\n", f.name)
		printLocation(f)
		fmt.Println("      Not tracked by Chai; sync will not overwrite it.")
		fmt.Println("      Move or remove it, then run chai sync.")
		return
	}
	fmt.Printf("INFO  Unmanaged skill: %s\n", f.name)
	printLocation(f)
	fmt.Println("      Not tracked by Chai; left untouched.")
	fmt.Println("      No action required unless you want Chai to manage it.")
}

func printLocation(f finding) {
	label := "Platform"
	if len(f.platforms) > 1 {
		label = "Platforms"
	}
	fmt.Printf("      %s: %s\n      Path: %s\n", label, strings.Join(f.platforms, ", "), f.path)
}
