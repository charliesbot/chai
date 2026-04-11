package resolve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathWithHome_Tilde(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"~", "/home/test"},
		{"~/foo", "/home/test/foo"},
		{"~/dotfiles/ai/agents.md", "/home/test/dotfiles/ai/agents.md"},
	}

	for _, tt := range tests {
		got, err := PathWithHome(tt.raw, "/home/test")
		if err != nil {
			t.Errorf("PathWithHome(%q): unexpected error: %v", tt.raw, err)
			continue
		}
		if got != tt.want {
			t.Errorf("PathWithHome(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestPathWithHome_DepRef(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"@workspace", "/home/test/.chai/deps/workspace"},
		{"@workspace/skills/foo", "/home/test/.chai/deps/workspace/skills/foo"},
		{"@angular-skills/skills/*", "/home/test/.chai/deps/angular-skills/skills/*"},
	}

	for _, tt := range tests {
		got, err := PathWithHome(tt.raw, "/home/test")
		if err != nil {
			t.Errorf("PathWithHome(%q): unexpected error: %v", tt.raw, err)
			continue
		}
		if got != tt.want {
			t.Errorf("PathWithHome(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestPathWithHome_Absolute(t *testing.T) {
	got, err := PathWithHome("/absolute/path", "/home/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/absolute/path" {
		t.Errorf("got %q, want %q", got, "/absolute/path")
	}
}

func TestPathWithHome_Empty(t *testing.T) {
	_, err := PathWithHome("", "/home/test")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestGlobWithHome(t *testing.T) {
	// Create a temp dir structure to glob against
	home := t.TempDir()
	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	for _, name := range []string{"web-dev", "android-dev", "slidev"} {
		dir := filepath.Join(skillsDir, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("creating dir: %v", err)
		}
	}

	matches, err := GlobWithHome("~/dotfiles/ai/skills/*", home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(matches) != 3 {
		t.Errorf("got %d matches, want 3: %v", len(matches), matches)
	}
}

func TestGlobWithHome_DepRef(t *testing.T) {
	home := t.TempDir()
	depSkills := filepath.Join(home, ".chai", "deps", "workspace", "skills")
	for _, name := range []string{"skill-a", "skill-b"} {
		if err := os.MkdirAll(filepath.Join(depSkills, name), 0755); err != nil {
			t.Fatalf("creating dir: %v", err)
		}
	}

	matches, err := GlobWithHome("@workspace/skills/*", home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(matches) != 2 {
		t.Errorf("got %d matches, want 2: %v", len(matches), matches)
	}
}

func TestGlobWithHome_NoMatches(t *testing.T) {
	home := t.TempDir()

	matches, err := GlobWithHome("~/nonexistent/*", home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(matches) != 0 {
		t.Errorf("got %d matches, want 0: %v", len(matches), matches)
	}
}
