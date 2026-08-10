package githubskill

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path"
	"sort"
	"strings"

	"github.com/charliesbot/chai/internal/skill"
)

type TreeEntry struct {
	Mode string
	Type string
	OID  string
	Path string
}

type Candidate struct {
	Name        string
	Description string
	SkillFile   string
	Directory   string
}

type Problem struct {
	Path string
	Err  error
}

func (p Problem) Error() string {
	return fmt.Sprintf("%s: %v", p.Path, p.Err)
}

type Discovery struct {
	Candidates []Candidate
	Problems   []Problem
	Entries    []TreeEntry
}

func Discover(ctx context.Context, cloneURL, repositoryDir string) (Discovery, error) {
	cmd := exec.CommandContext(ctx, "git", "clone", "--quiet", "--depth=1", "--filter=blob:none", "--no-checkout", "--", cloneURL, repositoryDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Discovery{}, fmt.Errorf("cloning GitHub skill source: %s: %w", strings.TrimSpace(string(output)), err)
	}
	cloneMessage := strings.ToLower(string(output))
	if strings.Contains(cloneMessage, "filtering not recognized") || strings.Contains(cloneMessage, "does not support filter") {
		return Discovery{}, fmt.Errorf("remote does not support the required partial clone; refusing full clone fallback")
	}
	if err := verifyPartialClone(ctx, repositoryDir); err != nil {
		return Discovery{}, err
	}

	output, err = gitOutput(ctx, repositoryDir, "ls-tree", "-r", "-z", "--full-tree", "HEAD")
	if err != nil {
		return Discovery{}, err
	}
	entries, err := parseTree(output)
	if err != nil {
		return Discovery{}, err
	}
	result := Discovery{Entries: entries}
	candidateEntries := skillEntries(entries)
	for _, entry := range candidateEntries {
		if entry.Mode != "100644" && entry.Mode != "100755" {
			result.Problems = append(result.Problems, Problem{Path: entry.Path, Err: fmt.Errorf("SKILL.md must be a regular file")})
			continue
		}
		directory := path.Dir(entry.Path)
		if directory == "." {
			result.Problems = append(result.Problems, Problem{Path: entry.Path, Err: fmt.Errorf("root-level remote skills are not supported")})
			continue
		}
		data, err := gitOutput(ctx, repositoryDir, "cat-file", "blob", entry.OID)
		if err != nil {
			return Discovery{}, err
		}
		metadata, err := skill.ParseMetadata(data)
		if err != nil {
			result.Problems = append(result.Problems, Problem{Path: entry.Path, Err: err})
			continue
		}
		result.Candidates = append(result.Candidates, Candidate{
			Name: metadata.Name, Description: metadata.Description,
			SkillFile: entry.Path, Directory: directory,
		})
	}
	sort.Slice(result.Candidates, func(i, j int) bool {
		if result.Candidates[i].Name == result.Candidates[j].Name {
			return result.Candidates[i].SkillFile < result.Candidates[j].SkillFile
		}
		return result.Candidates[i].Name < result.Candidates[j].Name
	})
	sort.Slice(result.Problems, func(i, j int) bool { return result.Problems[i].Path < result.Problems[j].Path })
	return result, nil
}

func skillEntries(entries []TreeEntry) []TreeEntry {
	standard := make([]TreeEntry, 0)
	all := make([]TreeEntry, 0)
	for _, entry := range entries {
		if path.Base(entry.Path) != "SKILL.md" {
			continue
		}
		all = append(all, entry)
		parts := strings.Split(entry.Path, "/")
		if len(parts) >= 3 && len(parts) <= 4 && parts[0] == "skills" {
			standard = append(standard, entry)
		}
	}
	if len(standard) > 0 {
		return standard
	}
	return all
}

func verifyPartialClone(ctx context.Context, repositoryDir string) error {
	promisor, err := gitOutput(ctx, repositoryDir, "config", "--get", "remote.origin.promisor")
	if err != nil || strings.TrimSpace(string(promisor)) != "true" {
		return fmt.Errorf("remote does not support the required partial clone; refusing full clone fallback")
	}
	filter, err := gitOutput(ctx, repositoryDir, "config", "--get", "remote.origin.partialclonefilter")
	if err != nil || strings.TrimSpace(string(filter)) != "blob:none" {
		return fmt.Errorf("remote did not preserve the required blob:none partial clone filter")
	}
	return nil
}

func parseTree(output []byte) ([]TreeEntry, error) {
	records := bytes.Split(output, []byte{0})
	entries := make([]TreeEntry, 0, len(records)-1)
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		separator := bytes.IndexByte(record, '\t')
		if separator < 0 {
			return nil, fmt.Errorf("parsing Git tree entry %q: missing path separator", record)
		}
		fields := strings.Fields(string(record[:separator]))
		if len(fields) != 3 {
			return nil, fmt.Errorf("parsing Git tree entry %q: invalid metadata", record)
		}
		entries = append(entries, TreeEntry{Mode: fields[0], Type: fields[1], OID: fields[2], Path: string(record[separator+1:])})
	}
	return entries, nil
}

func gitOutput(ctx context.Context, repositoryDir string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", repositoryDir}, args...)
	output, err := exec.CommandContext(ctx, "git", commandArgs...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("running git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), err)
	}
	return output, nil
}
