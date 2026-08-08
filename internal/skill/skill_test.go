package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverLocal(t *testing.T) {
	home := t.TempDir()
	base := filepath.Join(home, "project")
	collection := filepath.Join(base, "skills")
	writeSkill(t, filepath.Join(collection, "folder-name"), "declared-name")
	writeSkill(t, filepath.Join(collection, "another-folder"), "alpha")
	writeSkill(t, filepath.Join(collection, "nested", "ignored"), "ignored")

	sources, err := DiscoverLocal([]string{"./skills"}, base, home)
	if err != nil {
		t.Fatalf("discovering collection: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("sources = %v, want two immediate child skills", sources)
	}
	if sources[0].Name != "alpha" || sources[1].Name != "declared-name" || sources[1].Path != filepath.Join(collection, "folder-name") {
		t.Errorf("sources are not ordered by declared name: %+v", sources)
	}
}

func TestDiscoverLocal_SingleSkillAndTilde(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "my-skill")
	writeSkill(t, root, "single-skill")

	sources, err := DiscoverLocal([]string{"~/my-skill"}, t.TempDir(), home)
	if err != nil {
		t.Fatalf("discovering single skill: %v", err)
	}
	if len(sources) != 1 || sources[0].Name != "single-skill" || sources[0].Path != root {
		t.Fatalf("sources = %+v", sources)
	}
}

func TestDiscoverLocal_RejectsInvalidAndDuplicateNames(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "missing frontmatter",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "broken", "SKILL.md"), "body only")
			},
			want: "missing YAML frontmatter",
		},
		{
			name: "missing name",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "broken", "SKILL.md"), "---\ndescription: none\n---\nBody\n")
			},
			want: "missing required frontmatter field: name",
		},
		{
			name: "malformed yaml",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "broken", "SKILL.md"), "---\nname: [\n---\nBody\n")
			},
			want: "parsing YAML frontmatter",
		},
		{
			name: "invalid name",
			setup: func(t *testing.T, root string) {
				writeSkill(t, filepath.Join(root, "broken"), "Bad_Name")
			},
			want: `invalid skill name "Bad_Name"`,
		},
		{
			name: "whitespace padded name",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "broken", "SKILL.md"), "---\nname: \" padded-name \"\n---\nBody\n")
			},
			want: `invalid skill name " padded-name "`,
		},
		{
			name: "non-string name",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "broken", "SKILL.md"), "---\nname: true\n---\nBody\n")
			},
			want: "frontmatter field name must be a string",
		},
		{
			name: "malformed closing delimiter",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "broken", "SKILL.md"), "---\nname: skill\n---oops\nBody\n")
			},
			want: "unterminated YAML frontmatter",
		},
		{
			name: "duplicate name",
			setup: func(t *testing.T, root string) {
				writeSkill(t, filepath.Join(root, "one"), "shared")
				writeSkill(t, filepath.Join(root, "two"), "shared")
			},
			want: "duplicate local skill name conflicts: \"shared\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)
			_, err := DiscoverLocal([]string{root}, root, root)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
			if tt.name == "duplicate name" && (!strings.Contains(err.Error(), filepath.Join(root, "one")) || !strings.Contains(err.Error(), filepath.Join(root, "two"))) {
				t.Errorf("duplicate error does not contain both source paths: %v", err)
			}
		})
	}
}

func TestValidName_LengthBoundary(t *testing.T) {
	if !ValidName(strings.Repeat("a", 64)) || ValidName(strings.Repeat("a", 65)) {
		t.Fatal("ValidName should accept 64 characters and reject 65")
	}
}

func TestParseMetadata(t *testing.T) {
	metadata, err := ParseMetadata([]byte("---\nname: test-skill\ndescription: A useful skill\n---\nBody\n"))
	if err != nil {
		t.Fatalf("ParseMetadata: %v", err)
	}
	if metadata.Name != "test-skill" || metadata.Description != "A useful skill" {
		t.Fatalf("metadata = %+v", metadata)
	}
	if _, err := ParseMetadata([]byte("---\nname: test-skill\ndescription: true\n---\n")); err == nil {
		t.Fatal("non-string description should fail")
	}
}

func writeSkill(t *testing.T, dir, name string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: "+name+"\n---\nBody\n")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
