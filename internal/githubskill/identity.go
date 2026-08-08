package githubskill

import (
	"fmt"
	"path/filepath"
	"strings"
)

const githubURLPrefix = "https://github.com/"

type Identity struct {
	url        string
	owner      string
	repository string
}

func ParseCanonical(raw string) (Identity, error) {
	if !strings.HasPrefix(raw, githubURLPrefix) {
		return Identity{}, fmt.Errorf("GitHub skill URL must use canonical https://github.com/owner/repo form")
	}
	parts := strings.Split(strings.TrimPrefix(raw, githubURLPrefix), "/")
	if len(parts) != 2 || !validComponent(parts[0]) || !validComponent(parts[1]) || strings.HasSuffix(parts[1], ".git") {
		return Identity{}, fmt.Errorf("invalid canonical GitHub skill URL %q", raw)
	}
	return Identity{url: raw, owner: parts[0], repository: parts[1]}, nil
}

func (id Identity) URL() string {
	return id.url
}

func (id Identity) Owner() string {
	return id.owner
}

func (id Identity) Repository() string {
	return id.repository
}

func CacheDir(home string, id Identity) string {
	return filepath.Join(home, ".chai", "sources", "github.com", id.owner, id.repository)
}

func validComponent(component string) bool {
	if component == "" || component == "." || component == ".." || component != strings.ToLower(component) {
		return false
	}
	for _, r := range component {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}
