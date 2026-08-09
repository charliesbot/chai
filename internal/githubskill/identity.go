package githubskill

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

const githubURLPrefix = "https://github.com/"

type Identity struct {
	url        string
	owner      string
	repository string
}

func ParseInput(raw string) (Identity, error) {
	if !strings.Contains(raw, "://") {
		parts := strings.Split(raw, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return Identity{}, fmt.Errorf("GitHub source must be owner/repo or an HTTPS GitHub URL")
		}
		parts[1] = strings.TrimSuffix(parts[1], ".git")
		return canonicalIdentity(parts[0], parts[1])
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return Identity{}, fmt.Errorf("GitHub source URL must be public https://github.com/owner/repo without credentials, query, or fragment")
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 2 {
		return Identity{}, fmt.Errorf("GitHub source URL must contain exactly an owner and repository")
	}
	parts[1] = strings.TrimSuffix(parts[1], ".git")
	return canonicalIdentity(parts[0], parts[1])
}

func canonicalIdentity(owner, repository string) (Identity, error) {
	owner = strings.ToLower(owner)
	repository = strings.ToLower(repository)
	if !validComponent(owner) || !validComponent(repository) {
		return Identity{}, fmt.Errorf("invalid GitHub owner or repository")
	}
	canonical := githubURLPrefix + owner + "/" + repository
	return Identity{url: canonical, owner: owner, repository: repository}, nil
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
