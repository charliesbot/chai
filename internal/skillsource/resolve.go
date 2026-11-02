// Package skillsource resolves the sources configured in a chai manifest.
package skillsource

import (
	"errors"
	"sort"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
	"github.com/charliesbot/chai/internal/skill"
)

// Resolve returns every configured local and cached GitHub skill source. It
// validates the complete configured set before reading remote caches so callers
// can safely inspect the same source set that sync would install.
func Resolve(cfg *config.Config, home string) ([]skill.Source, error) {
	resolved, err := skill.DiscoverLocal(cfg.Skills.Local, home, home)
	if err != nil {
		return nil, err
	}
	configured := append([]skill.Source(nil), resolved...)
	for _, remote := range cfg.Skills.GitHub {
		for _, name := range remote.Include {
			configured = append(configured, skill.Source{Name: name, Path: remote.URL})
		}
	}
	if err := skill.ValidateUniqueNames(configured); err != nil {
		return nil, err
	}

	var sourceErrors []error
	for _, remote := range cfg.Skills.GitHub {
		id, err := githubskill.ParseCanonical(remote.URL)
		if err != nil {
			return nil, err
		}
		cached, err := githubskill.ResolveCached(home, id, remote.Include)
		if err != nil {
			sourceErrors = append(sourceErrors, err)
			continue
		}
		resolved = append(resolved, cached...)
	}
	if len(sourceErrors) > 0 {
		return nil, errors.Join(sourceErrors...)
	}
	sort.Slice(resolved, func(i, j int) bool { return resolved[i].Name < resolved[j].Name })
	return resolved, nil
}
