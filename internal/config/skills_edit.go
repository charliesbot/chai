package config

import (
	"bytes"
	"fmt"
	"os"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// UpdateSkillsAtomic replaces only Chai's skills configuration, preserving all
// other user-authored manifest content.
func UpdateSkillsAtomic(path string, cfg *Config) error {
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
