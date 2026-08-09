package resolve

import (
	"fmt"
	"path/filepath"
	"strings"
)

const depsDir = ".chai/deps"

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
