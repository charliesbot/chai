package resolve

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const depsDir = ".chai/deps"

// Path resolves ~ and @name prefixes in a single path.
// It does not expand globs.
func Path(raw string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return PathWithHome(raw, home)
}

// PathWithHome resolves ~ and @name prefixes using the given home directory.
func PathWithHome(raw, home string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("empty path")
	}

	if raw == "~" {
		return home, nil
	}
	if strings.HasPrefix(raw, "~/") {
		return filepath.Join(home, raw[2:]), nil
	}

	if strings.HasPrefix(raw, "@") {
		rest := raw[1:]
		slash := strings.IndexByte(rest, '/')
		if slash == -1 {
			// @name with no trailing path
			name := rest
			return filepath.Join(home, depsDir, name), nil
		}
		name := rest[:slash]
		tail := rest[slash+1:]
		return filepath.Join(home, depsDir, name, tail), nil
	}

	return raw, nil
}

// Glob resolves a path (expanding ~ and @name) then expands glob patterns.
// Returns the matched file paths sorted by filepath.Glob's default order.
func Glob(pattern string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving home directory: %w", err)
	}
	return GlobWithHome(pattern, home)
}

// GlobWithHome resolves a path then expands glob patterns using the given home directory.
func GlobWithHome(pattern, home string) ([]string, error) {
	resolved, err := PathWithHome(pattern, home)
	if err != nil {
		return nil, err
	}

	matches, err := filepath.Glob(resolved)
	if err != nil {
		return nil, fmt.Errorf("expanding glob %q: %w", resolved, err)
	}

	return matches, nil
}
