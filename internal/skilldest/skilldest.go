// Package skilldest decides which configured skill destinations chai may write.
// Sync, add, and doctor all use it so they agree on what blocks a sync.
package skilldest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/skill"
)

// Collision is an existing destination for a configured skill that chai does
// not own, so sync refuses to overwrite it.
type Collision struct {
	Name string
	Path string
	// Platforms lists every platform whose skills directory holds Path, in the
	// order the platforms were given. Platforms can share a skills directory.
	Platforms []string
}

// Unowned reports whether something exists at path that chai does not own.
// A path recorded in hashDB is owned by chai and is never inspected on disk.
func Unowned(path string, hashDB hash.DB) (bool, error) {
	if _, managed := hashDB[path]; managed {
		return false, nil
	}
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("checking skill destination %s: %w", path, err)
}

// Collisions returns the unowned existing destinations for the named skills,
// sorted by path.
func Collisions(home string, platforms []platform.Platform, names []string, hashDB hash.DB) ([]Collision, error) {
	checked := make(map[string]bool)
	byPath := make(map[string]*Collision)
	for _, p := range platforms {
		for _, name := range names {
			path := filepath.Join(home, p.SkillsDir, skill.DirectoryName(name))
			if checked[path] {
				if collision, ok := byPath[path]; ok {
					collision.Platforms = append(collision.Platforms, p.Name)
				}
				continue
			}
			checked[path] = true
			unowned, err := Unowned(path, hashDB)
			if err != nil {
				return nil, err
			}
			if unowned {
				byPath[path] = &Collision{Name: name, Path: path, Platforms: []string{p.Name}}
			}
		}
	}

	collisions := make([]Collision, 0, len(byPath))
	for _, collision := range byPath {
		collisions = append(collisions, *collision)
	}
	sort.Slice(collisions, func(i, j int) bool { return collisions[i].Path < collisions[j].Path })
	return collisions, nil
}

// Validate reports existing selected-skill paths that chai cannot safely
// overwrite because they are absent from its ownership DB.
func Validate(names []string, home string, platformNames []string) error {
	hashDB, err := hash.Load(home)
	if err != nil {
		return err
	}
	collisions, err := Collisions(home, platform.ForNames(platformNames), names, hashDB)
	if err != nil {
		return err
	}
	if len(collisions) == 0 {
		return nil
	}
	paths := make([]string, len(collisions))
	for i, collision := range collisions {
		paths[i] = collision.Path
	}
	return fmt.Errorf(
		"existing skill destinations are not managed by chai:\n  %s\nmove or remove these directories, then retry",
		strings.Join(paths, "\n  "),
	)
}
