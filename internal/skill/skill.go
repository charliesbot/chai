package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Source struct {
	Name string
	Path string
}

type Metadata struct {
	Name        string
	Description string
}

func ValidName(name string) bool {
	return len(name) <= 64 && namePattern.MatchString(name)
}

func DiscoverLocal(roots []string, baseDir, home string) ([]Source, error) {
	var sources []Source
	for _, rawRoot := range roots {
		root := resolveLocalPath(rawRoot, baseDir, home)
		info, err := os.Stat(root)
		if err != nil {
			return nil, fmt.Errorf("reading local skill source %q: %w", rawRoot, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("local skill source %q is not a directory", rawRoot)
		}

		if source, found, err := sourceFromDir(root); err != nil {
			return nil, err
		} else if found {
			sources = append(sources, source)
			continue
		}

		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, fmt.Errorf("reading local skill collection %s: %w", root, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			source, found, err := sourceFromDir(filepath.Join(root, entry.Name()))
			if err != nil {
				return nil, err
			}
			if found {
				sources = append(sources, source)
			}
		}
	}

	if err := rejectDuplicateNames(sources); err != nil {
		return nil, err
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Name < sources[j].Name })
	return sources, nil
}

func resolveLocalPath(raw, baseDir, home string) string {
	switch {
	case raw == "~":
		return home
	case strings.HasPrefix(raw, "~/"):
		return filepath.Join(home, raw[2:])
	case filepath.IsAbs(raw):
		return filepath.Clean(raw)
	default:
		return filepath.Clean(filepath.Join(baseDir, raw))
	}
}

func sourceFromDir(dir string) (Source, bool, error) {
	path := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Source{}, false, nil
	}
	if err != nil {
		return Source{}, false, fmt.Errorf("reading %s: %w", path, err)
	}

	metadata, err := ParseMetadata(data)
	if err != nil {
		return Source{}, false, fmt.Errorf("%s: %w", path, err)
	}
	return Source{Name: metadata.Name, Path: dir}, true, nil
}

func ParseMetadata(data []byte) (Metadata, error) {
	text := strings.TrimPrefix(string(data), "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Metadata{}, fmt.Errorf("missing YAML frontmatter")
	}
	lines := strings.Split(text, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return Metadata{}, fmt.Errorf("unterminated YAML frontmatter")
	}

	var metadata struct {
		Name        yaml.Node `yaml:"name"`
		Description yaml.Node `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &metadata); err != nil {
		return Metadata{}, fmt.Errorf("parsing YAML frontmatter: %w", err)
	}
	if metadata.Name.Kind != 0 && metadata.Name.Tag != "!!str" {
		return Metadata{}, fmt.Errorf("frontmatter field name must be a string")
	}
	name := metadata.Name.Value
	if name == "" {
		return Metadata{}, fmt.Errorf("missing required frontmatter field: name")
	}
	if !ValidName(name) {
		return Metadata{}, fmt.Errorf("invalid skill name %q", name)
	}
	if metadata.Description.Kind != 0 && metadata.Description.Tag != "!!str" {
		return Metadata{}, fmt.Errorf("frontmatter field description must be a string")
	}
	return Metadata{Name: name, Description: strings.TrimSpace(metadata.Description.Value)}, nil
}

func rejectDuplicateNames(sources []Source) error {
	locations := make(map[string][]string)
	for _, source := range sources {
		locations[source.Name] = append(locations[source.Name], source.Path)
	}
	var conflicts []string
	for name, paths := range locations {
		if len(paths) > 1 {
			sort.Strings(paths)
			conflicts = append(conflicts, fmt.Sprintf("%q: %s", name, strings.Join(paths, ", ")))
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	sort.Strings(conflicts)
	return fmt.Errorf("duplicate local skill name conflicts: %s", strings.Join(conflicts, "; "))
}
