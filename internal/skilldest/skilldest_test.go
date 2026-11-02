package skilldest

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
)

func TestCollisionsAttributesSharedDestinationToEveryPlatform(t *testing.T) {
	home := t.TempDir()
	shared := filepath.Join(home, ".agents", "skills", "chosen")
	mkdir(t, shared)
	platforms := []platform.Platform{
		{Name: "Codex", SkillsDir: ".agents/skills"},
		{Name: "Other", SkillsDir: ".agents/skills"},
	}

	got, err := Collisions(home, platforms, []string{"chosen"}, hash.DB{})
	if err != nil {
		t.Fatal(err)
	}
	want := []Collision{{Name: "chosen", Path: shared, Platforms: []string{"Codex", "Other"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Collisions = %+v, want %+v", got, want)
	}
}

func TestCollisionsUsesNamespacedDirectoryName(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "skills", "team-review")
	mkdir(t, path)
	platforms := []platform.Platform{{Name: "Claude", SkillsDir: ".claude/skills"}}

	got, err := Collisions(home, platforms, []string{"team::review"}, hash.DB{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != path || got[0].Name != "team::review" {
		t.Fatalf("Collisions = %+v", got)
	}
}

func TestCollisionsNeverInspectsManagedDestinations(t *testing.T) {
	home := t.TempDir()
	loop := filepath.Join(home, ".claude", "skills", "chosen")
	mkdir(t, filepath.Dir(loop))
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatal(err)
	}
	platforms := []platform.Platform{{Name: "Claude", SkillsDir: ".claude/skills"}}

	got, err := Collisions(home, platforms, []string{"chosen"}, hash.DB{loop: "h"})
	if err != nil || len(got) != 0 {
		t.Fatalf("Collisions = %+v, err = %v", got, err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}
