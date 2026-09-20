package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/joho/godotenv"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var headerName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

// ValidateMCP validates the shared transport schema without resolving secrets.
func ValidateMCP(servers map[string]MCP) error { return validateMCP(servers, true) }

func validateMCP(servers map[string]MCP, allowReferences bool) error {
	for _, name := range sortedKeys(servers) {
		m := servers[name]
		if (m.Command == "") == (m.URL == "") {
			return fmt.Errorf("mcp.%s: set exactly one of command or url", name)
		}
		if m.URL != "" {
			if len(m.Args) > 0 || len(m.Env) > 0 || m.CWD != "" {
				return fmt.Errorf("mcp.%s: url cannot be combined with args, env, or cwd", name)
			}
			// Variables are checked after expansion, before any sync writes.
			if !allowReferences || !strings.Contains(m.URL, "${") {
				u, err := url.Parse(m.URL)
				if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" {
					return fmt.Errorf("mcp.%s.url: expected an HTTP(S) URL without user info or fragment", name)
				}
			}
		} else if len(m.Headers) > 0 {
			return fmt.Errorf("mcp.%s: headers require url", name)
		}
		for _, key := range sortedKeys(m.Headers) {
			if !headerName.MatchString(key) || strings.ContainsAny(m.Headers[key], "\r\n\x00") {
				return fmt.Errorf("mcp.%s.headers: invalid header name or value", name)
			}
		}
	}
	return nil
}

// ResolvedMCP keeps secret-bearing values separate from the printable preview.
// Neither map aliases the manifest; only Values may be written to target configs.
type ResolvedMCP struct {
	Values  map[string]MCP
	Preview map[string]MCP
}

// ResolveMCP reads one private dotenv file and expands MCP string values only.
// It never changes the process environment or the source manifest.
func ResolveMCP(servers map[string]MCP, home string) (ResolvedMCP, error) {
	result := ResolvedMCP{Values: make(map[string]MCP), Preview: make(map[string]MCP)}
	if len(servers) == 0 {
		return result, nil
	}
	if err := ValidateMCP(servers); err != nil {
		return result, err
	}
	path := filepath.Join(home, ".config", "chai", ".env")
	values, err := readPrivateEnv(path)
	if err != nil {
		return result, err
	}
	lookup := func(key string) (string, bool) {
		if value, ok := os.LookupEnv(key); ok {
			return value, true
		}
		value, ok := values[key]
		return value, ok
	}
	for _, name := range sortedKeys(servers) {
		m := servers[name]
		resolved, err := expandMCP(m, lookup)
		if err != nil {
			return result, fmt.Errorf("mcp.%s: %w", name, err)
		}
		result.Values[name] = resolved
		result.Preview[name] = previewMCP(m, resolved)
	}
	if err := validateMCP(result.Values, false); err != nil {
		return result, err
	}
	return result, nil
}

func expandMCP(m MCP, lookup func(string) (string, bool)) (MCP, error) {
	resolved := m
	var err error
	resolved.Command, err = expandMCPValue(m.Command, lookup)
	if err != nil {
		return MCP{}, err
	}
	resolved.URL, err = expandMCPValue(m.URL, lookup)
	if err != nil {
		return MCP{}, err
	}
	resolved.CWD, err = expandMCPValue(m.CWD, lookup)
	if err != nil {
		return MCP{}, err
	}
	resolved.Args = make([]string, len(m.Args))
	for i, arg := range m.Args {
		resolved.Args[i], err = expandMCPValue(arg, lookup)
		if err != nil {
			return MCP{}, err
		}
	}
	resolved.Env, err = expandMCPMap(m.Env, lookup)
	if err != nil {
		return MCP{}, err
	}
	resolved.Headers, err = expandMCPMap(m.Headers, lookup)
	if err != nil {
		return MCP{}, err
	}
	return resolved, nil
}

func expandMCPMap(values map[string]string, lookup func(string) (string, bool)) (map[string]string, error) {
	resolved := make(map[string]string)
	for _, key := range sortedKeys(values) {
		value, err := expandMCPValue(values[key], lookup)
		if err != nil {
			return nil, err
		}
		resolved[key] = value
	}
	return resolved, nil
}

func previewMCP(original, resolved MCP) MCP {
	preview := resolved
	preview.Command = previewMCPValue(original.Command, resolved.Command)
	preview.URL = previewMCPValue(original.URL, resolved.URL)
	preview.CWD = previewMCPValue(original.CWD, resolved.CWD)
	preview.Args = make([]string, len(resolved.Args))
	for i, arg := range original.Args {
		preview.Args[i] = previewMCPValue(arg, resolved.Args[i])
	}
	preview.Env = make(map[string]string)
	for key := range original.Env {
		preview.Env[key] = "<redacted>"
	}
	preview.Headers = make(map[string]string)
	for key := range original.Headers {
		preview.Headers[key] = "<redacted>"
	}
	return preview
}

func previewMCPValue(original, resolved string) string {
	if strings.Contains(original, "${") {
		return "<redacted>"
	}
	return resolved
}

func readPrivateEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening Chai environment file: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("checking Chai environment file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("Chai environment file must be a regular, owner-only file (chmod 600 %s)", path)
	}
	values, err := godotenv.Parse(f)
	if err != nil {
		return nil, fmt.Errorf("invalid Chai environment file %s (contents omitted)", path)
	}
	return values, nil
}

func expandMCPValue(raw string, lookup func(string) (string, bool)) (string, error) {
	var result strings.Builder
	for len(raw) > 0 {
		if strings.HasPrefix(raw, "$$") {
			result.WriteByte('$')
			raw = raw[2:]
			continue
		}
		if !strings.HasPrefix(raw, "${") {
			result.WriteByte(raw[0])
			raw = raw[1:]
			continue
		}
		end := strings.IndexByte(raw, '}')
		if end < 0 || !envName.MatchString(raw[2:end]) {
			return "", fmt.Errorf("invalid environment reference; use ${NAME}")
		}
		name := raw[2:end]
		value, ok := lookup(name)
		if !ok || value == "" {
			return "", fmt.Errorf("environment variable %s is missing or empty", name)
		}
		result.WriteString(value)
		raw = raw[end+1:]
	}
	return result.String(), nil
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
