package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// NormalizeLocalSkillPath converts a local skill source to its manifest form.
func NormalizeLocalSkillPath(raw, home string) (string, error) {
	if !isLocalSkillInput(raw) {
		return "", fmt.Errorf("local source must begin with /, ~/, ./, or ../")
	}
	switch {
	case raw == "~":
		return "~", nil
	case strings.HasPrefix(raw, "~/"):
		clean := filepath.Clean(raw[2:])
		if clean == "." {
			return "~", nil
		}
		return "~/" + filepath.ToSlash(clean), nil
	case filepath.IsAbs(raw):
		clean := filepath.Clean(raw)
		relative, err := filepath.Rel(home, clean)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			if relative == "." {
				return "~", nil
			}
			return "~/" + filepath.ToSlash(relative), nil
		}
		return clean, nil
	case strings.HasPrefix(raw, "./"):
		clean := filepath.Clean(raw)
		if clean == "." {
			return "./.", nil
		}
		return "./" + filepath.ToSlash(strings.TrimPrefix(clean, "./")), nil
	default:
		clean := filepath.Clean(raw)
		if clean == ".." {
			return "../.", nil
		}
		return filepath.ToSlash(clean), nil
	}
}

func isLocalSkillInput(source string) bool {
	return source == "~" || strings.HasPrefix(source, "~/") || filepath.IsAbs(source) ||
		strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../")
}

// AddLocalSkillSourceAtomic records one normalized local source without
// exposing manifest formatting or write ordering to the caller.
func AddLocalSkillSourceAtomic(
	path string,
	cfg *Config,
	source string,
	home string,
	validateCandidate func(*Config) error,
) error {
	candidate := cloneConfig(cfg)
	found := false
	for _, existing := range candidate.Skills.Local {
		canonical, err := NormalizeLocalSkillPath(existing, home)
		if err == nil && canonical == source {
			found = true
			break
		}
	}
	if !found {
		candidate.Skills.Local = append(candidate.Skills.Local, source)
		sort.Strings(candidate.Skills.Local)
	}
	return commitSkillsMutation(path, cfg, candidate, validateCandidate)
}

// ReconcileGitHubSkillSourceAtomic records the selected skills for one GitHub
// source without exposing manifest formatting or write ordering to the caller.
func ReconcileGitHubSkillSourceAtomic(
	path string,
	cfg *Config,
	source GitHubSkills,
	validateCandidate func(*Config) error,
) error {
	candidate := cloneConfig(cfg)
	source.Include = sortedUniqueStrings(source.Include)
	replaced := false
	for i := range candidate.Skills.GitHub {
		if candidate.Skills.GitHub[i].URL == source.URL {
			candidate.Skills.GitHub[i] = source
			replaced = true
			break
		}
	}
	if !replaced {
		candidate.Skills.GitHub = append(candidate.Skills.GitHub, source)
	}
	sort.Slice(candidate.Skills.GitHub, func(i, j int) bool {
		return candidate.Skills.GitHub[i].URL < candidate.Skills.GitHub[j].URL
	})
	return commitSkillsMutation(path, cfg, candidate, validateCandidate)
}

func cloneConfig(cfg *Config) *Config {
	candidate := *cfg
	candidate.Platforms = append([]string(nil), cfg.Platforms...)
	candidate.Instructions = append([]string(nil), cfg.Instructions...)
	if cfg.Deps != nil {
		candidate.Deps = make(map[string]Dep, len(cfg.Deps))
		for name, dep := range cfg.Deps {
			candidate.Deps[name] = dep
		}
	}
	candidate.Skills.Local = append([]string(nil), cfg.Skills.Local...)
	candidate.Skills.GitHub = append([]GitHubSkills(nil), cfg.Skills.GitHub...)
	for i := range candidate.Skills.GitHub {
		candidate.Skills.GitHub[i].Include = append([]string(nil), cfg.Skills.GitHub[i].Include...)
	}
	candidate.Subagents.Paths = append([]string(nil), cfg.Subagents.Paths...)
	if cfg.MCP != nil {
		candidate.MCP = make(map[string]MCP, len(cfg.MCP))
		for name, mcp := range cfg.MCP {
			mcp.Args = append([]string(nil), mcp.Args...)
			mcp.Env = cloneStringMap(mcp.Env)
			candidate.MCP[name] = mcp
		}
	}
	candidate.Antigravity.Plugins = cloneStringMap(cfg.Antigravity.Plugins)
	candidate.Droid.CustomModels = append([]CustomModel(nil), cfg.Droid.CustomModels...)
	return &candidate
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func sortedUniqueStrings(values []string) []string {
	unique := make(map[string]bool, len(values))
	for _, value := range values {
		unique[value] = true
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func commitSkillsMutation(
	path string,
	cfg *Config,
	candidate *Config,
	validateCandidate func(*Config) error,
) error {
	if err := validate(candidate); err != nil {
		return fmt.Errorf("validating config before write: %w", err)
	}
	if validateCandidate != nil {
		if err := validateCandidate(cloneConfig(candidate)); err != nil {
			return err
		}
	}
	if err := writeSkillsAtomic(path, candidate); err != nil {
		return err
	}
	*cfg = *candidate
	return nil
}

// writeSkillsAtomic replaces only Chai's skills configuration, preserving all
// other user-authored manifest content.
func writeSkillsAtomic(path string, cfg *Config) error {
	if err := validate(cfg); err != nil {
		return fmt.Errorf("validating config before write: %w", err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SaveAtomic(path, cfg)
		}
		return fmt.Errorf("reading config: %w", err)
	}
	section, err := encodeSkills(cfg.Skills)
	if err != nil {
		return err
	}
	updated, err := replaceSkillsExpressions(original, section)
	if err != nil {
		return fmt.Errorf("editing skills config: %w", err)
	}

	if _, err := load(path, updated); err != nil {
		return fmt.Errorf("validating updated config: %w", err)
	}
	return writeAtomic(path, updated)
}

func encodeSkills(skills Skills) ([]byte, error) {
	var output bytes.Buffer
	encoder := toml.NewEncoder(&output).SetArraysMultiline(true)
	if err := encoder.Encode(struct {
		Skills Skills `toml:"skills"`
	}{Skills: skills}); err != nil {
		return nil, fmt.Errorf("encoding skills config: %w", err)
	}
	return output.Bytes(), nil
}

type sourceRange struct {
	start int
	end   int
}

func replaceSkillsExpressions(input, replacement []byte) ([]byte, error) {
	var parser unstable.Parser
	parser.Reset(input)
	var ranges []sourceRange
	insideTable := false
	insideSkillsTable := false

	for parser.NextExpression() {
		node := parser.Expression()
		switch node.Kind {
		case unstable.Table, unstable.ArrayTable:
			insideTable = true
			insideSkillsTable = firstKey(node) == "skills"
			if insideSkillsTable {
				ranges = append(ranges, expressionRange(input, node))
			}
		case unstable.KeyValue:
			if insideSkillsTable {
				ranges[len(ranges)-1].end = expressionRange(input, node).end
				continue
			}
			if !insideTable && firstKey(node) == "skills" {
				ranges = append(ranges, expressionRange(input, node))
			}
		}
	}
	if err := parser.Error(); err != nil {
		return nil, err
	}
	return replaceRanges(input, replacement, ranges), nil
}

func firstKey(node *unstable.Node) string {
	key := node.Key()
	key.Next()
	return string(key.Node().Data)
}

func expressionRange(input []byte, node *unstable.Node) sourceRange {
	if node.Kind == unstable.Table || node.Kind == unstable.ArrayTable {
		key := node.Key()
		key.Next()
		keyStart := int(key.Node().Raw.Offset)
		start := bytes.LastIndexByte(input[:keyStart], '\n') + 1
		end := bytes.IndexByte(input[keyStart:], '\n')
		if end < 0 {
			end = len(input)
		} else {
			end += keyStart
		}
		return sourceRange{start: start, end: end}
	}
	start := int(node.Raw.Offset)
	return sourceRange{start: start, end: start + int(node.Raw.Length)}
}

func replaceRanges(input, replacement []byte, ranges []sourceRange) []byte {
	if len(ranges) == 0 {
		var output bytes.Buffer
		output.Write(input)
		if output.Len() > 0 && output.Bytes()[output.Len()-1] != '\n' {
			output.WriteByte('\n')
		}
		if output.Len() > 0 {
			output.WriteByte('\n')
		}
		output.Write(replacement)
		return output.Bytes()
	}

	ranges = coalesceWhitespaceSeparatedRanges(input, ranges)
	var output bytes.Buffer
	last := 0
	for i, current := range ranges {
		start := current.start
		end := current.end
		if i == 0 {
			end = skipWhitespace(input, end)
			output.Write(input[last:start])
			output.Write(bytes.TrimRight(replacement, "\n"))
			if end < len(input) {
				output.WriteString("\n\n")
			} else {
				output.WriteByte('\n')
			}
		} else {
			start = skipWhitespaceBackward(input, start, last)
			output.Write(input[last:start])
		}
		last = end
	}
	output.Write(input[last:])
	return output.Bytes()
}

func coalesceWhitespaceSeparatedRanges(input []byte, ranges []sourceRange) []sourceRange {
	merged := []sourceRange{ranges[0]}
	for _, current := range ranges[1:] {
		last := &merged[len(merged)-1]
		if skipWhitespace(input, last.end) == current.start {
			last.end = current.end
			continue
		}
		merged = append(merged, current)
	}
	return merged
}

func skipWhitespace(input []byte, start int) int {
	for start < len(input) {
		switch input[start] {
		case ' ', '\t', '\r', '\n':
			start++
		default:
			return start
		}
	}
	return start
}

func skipWhitespaceBackward(input []byte, start, limit int) int {
	for start > limit {
		switch input[start-1] {
		case ' ', '\t', '\r', '\n':
			start--
		default:
			return start
		}
	}
	return start
}
