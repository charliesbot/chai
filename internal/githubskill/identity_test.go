package githubskill

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCanonical(t *testing.T) {
	id, err := ParseCanonical("https://github.com/vercel-labs/agent-skills")
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}
	if id.Owner() != "vercel-labs" || id.Repository() != "agent-skills" || id.URL() != "https://github.com/vercel-labs/agent-skills" {
		t.Fatalf("identity = %+v", id)
	}

	wantCache := filepath.Join("/home/test", ".chai", "sources", "github.com", "vercel-labs", "agent-skills")
	if got := CacheDir("/home/test", id); got != wantCache {
		t.Fatalf("CacheDir = %q, want %q", got, wantCache)
	}
}

func TestParseCanonical_RejectsInvalid(t *testing.T) {
	for _, raw := range []string{
		"http://github.com/owner/repo",
		"https://github.com/Owner/repo",
		"https://github.com/owner/Repo",
		"https://github.com/owner/repo.git",
		"https://github.com/owner/repo/",
		"https://github.com/owner/repo/path",
		"https://github.com/owner/repo?ref=main",
		"https://user@github.com/owner/repo",
		"https://github.com/../repo",
	} {
		t.Run(strings.ReplaceAll(raw, "/", "_"), func(t *testing.T) {
			if _, err := ParseCanonical(raw); err == nil {
				t.Fatalf("ParseCanonical(%q) succeeded", raw)
			}
		})
	}
}

func TestParseInputCanonicalizesGitHubSources(t *testing.T) {
	for _, input := range []string{
		"Example/Skills",
		"Example/Skills.git",
		"https://github.com/Example/Skills/",
		"https://github.com/Example/Skills.git",
	} {
		id, err := ParseInput(input)
		if err != nil {
			t.Fatalf("ParseInput(%q): %v", input, err)
		}
		if id.URL() != "https://github.com/example/skills" {
			t.Fatalf("ParseInput(%q) = %q", input, id.URL())
		}
	}
}

func TestParseInputRejectsUnsupportedRemotes(t *testing.T) {
	for _, input := range []string{
		"http://github.com/example/skills",
		"https://github.com/example/skills/tree/main",
		"https://github.com/example/skills?ref=main",
		"git@example.com:example/skills.git",
	} {
		if _, err := ParseInput(input); err == nil {
			t.Fatalf("ParseInput(%q) should fail", input)
		}
	}
}
