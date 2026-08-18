package githubskill

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type MissingSkillsError struct {
	Names []string
}

func (err *MissingSkillsError) Error() string {
	if len(err.Names) == 1 {
		return fmt.Sprintf("remote skill %q was not found", err.Names[0])
	}
	return fmt.Sprintf("remote skills %s were not found", quotedNames(err.Names))
}

func quotedNames(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = fmt.Sprintf("%q", name)
	}
	return strings.Join(quoted, ", ")
}

func Materialize(ctx context.Context, repositoryDir string, discovery Discovery, names []string) (map[string]string, error) {
	candidates := make(map[string][]Candidate)
	for _, candidate := range discovery.Candidates {
		candidates[candidate.Name] = append(candidates[candidate.Name], candidate)
	}
	var missing []string
	for _, name := range names {
		if len(candidates[name]) == 0 {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		missing = sortedUniqueStrings(missing)
		return nil, &MissingSkillsError{Names: missing}
	}
	selected := make(map[string]string, len(names))
	var directories []string
	for _, name := range names {
		matches := candidates[name]
		if len(matches) > 1 {
			return nil, fmt.Errorf("remote skill %q is ambiguous", name)
		}
		if _, duplicate := selected[name]; duplicate {
			return nil, fmt.Errorf("remote skill %q was selected more than once", name)
		}
		selected[name] = matches[0].Directory
		directories = append(directories, matches[0].Directory)
	}
	if err := validateSelectedEntries(discovery.Entries, directories); err != nil {
		return nil, err
	}
	patterns, err := sparsePatterns(directories)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repositoryDir, "sparse-checkout", "set", "--no-cone", "--stdin")
	cmd.Stdin = bytes.NewReader(patterns)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("setting sparse skill paths: %s: %w", strings.TrimSpace(string(output)), err)
	}
	if _, err := gitOutput(ctx, repositoryDir, "checkout", "--quiet", "HEAD"); err != nil {
		return nil, err
	}
	if err := materializeSelectedSymlinks(ctx, repositoryDir, discovery.Entries, directories); err != nil {
		return nil, err
	}
	for _, directory := range directories {
		if err := validateMaterializedTree(repositoryDir, directory); err != nil {
			return nil, err
		}
	}
	return selected, nil
}

func sortedUniqueStrings(values []string) []string {
	sort.Strings(values)
	unique := values[:0]
	for _, value := range values {
		if len(unique) == 0 || unique[len(unique)-1] != value {
			unique = append(unique, value)
		}
	}
	return unique
}

func validateSelectedEntries(entries []TreeEntry, directories []string) error {
	for _, directory := range directories {
		if !safeGitPath(directory) {
			return fmt.Errorf("unsafe selected skill directory %q", directory)
		}
		prefix := directory + "/"
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Path, prefix) {
				continue
			}
			if !safeGitPath(entry.Path) {
				return fmt.Errorf("unsafe selected skill path %q", entry.Path)
			}
			switch entry.Mode {
			case "100644", "100755":
				if entry.Type != "blob" {
					return fmt.Errorf("selected skill %q contains non-blob regular mode at %q", directory, entry.Path)
				}
			case "120000":
				if entry.Type != "blob" {
					return fmt.Errorf("selected skill %q contains non-blob symbolic link mode at %q", directory, entry.Path)
				}
			case "160000":
				return fmt.Errorf("selected skill %q contains Git submodule %q", directory, entry.Path)
			default:
				return fmt.Errorf("selected skill %q contains unsupported mode %s at %q", directory, entry.Mode, entry.Path)
			}
		}
	}
	return nil
}

func materializeSelectedSymlinks(ctx context.Context, repositoryDir string, entries []TreeEntry, directories []string) error {
	entriesByPath := make(map[string]TreeEntry, len(entries))
	for _, entry := range entries {
		entriesByPath[entry.Path] = entry
	}
	for _, directory := range directories {
		prefix := directory + "/"
		for _, entry := range entries {
			if entry.Mode != "120000" || !strings.HasPrefix(entry.Path, prefix) {
				continue
			}
			targetBytes, err := gitOutput(ctx, repositoryDir, "cat-file", "blob", entry.OID)
			if err != nil {
				return fmt.Errorf("reading symbolic link %q: %w", entry.Path, err)
			}
			target := string(targetBytes)
			if target == "" || strings.HasPrefix(target, "/") || strings.ContainsAny(target, "\x00\n") {
				return fmt.Errorf("selected skill %q contains unsafe symbolic link %q", directory, entry.Path)
			}
			resolved := path.Clean(path.Join(path.Dir(entry.Path), target))
			if !safeGitPath(resolved) {
				return fmt.Errorf("selected skill %q contains symbolic link %q that escapes the repository", directory, entry.Path)
			}
			targetEntry, ok := entriesByPath[resolved]
			if !ok {
				return fmt.Errorf("selected skill %q contains symbolic link %q with missing or non-file target %q", directory, entry.Path, resolved)
			}
			if targetEntry.Type != "blob" || targetEntry.Mode != "100644" && targetEntry.Mode != "100755" {
				return fmt.Errorf("selected skill %q contains symbolic link %q to unsupported target %q", directory, entry.Path, resolved)
			}
			contents, err := gitOutput(ctx, repositoryDir, "cat-file", "blob", targetEntry.OID)
			if err != nil {
				return fmt.Errorf("reading symbolic link target %q: %w", resolved, err)
			}
			mode := fs.FileMode(0644)
			if targetEntry.Mode == "100755" {
				mode = 0755
			}
			if err := replaceSymlinkWithFile(repositoryDir, entry.Path, contents, mode); err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceSymlinkWithFile(repositoryDir, gitPath string, contents []byte, mode fs.FileMode) error {
	filePath := filepath.Join(repositoryDir, filepath.FromSlash(gitPath))
	temporary, err := os.CreateTemp(filepath.Dir(filePath), ".chai-symlink-*")
	if err != nil {
		return fmt.Errorf("creating replacement for symbolic link %q: %w", gitPath, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("writing replacement for symbolic link %q: %w", gitPath, err)
	}
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return fmt.Errorf("setting replacement mode for symbolic link %q: %w", gitPath, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("closing replacement for symbolic link %q: %w", gitPath, err)
	}
	if err := os.Rename(temporaryPath, filePath); err != nil {
		return fmt.Errorf("installing replacement for symbolic link %q: %w", gitPath, err)
	}
	return nil
}

func sparsePatterns(directories []string) ([]byte, error) {
	directories = append([]string(nil), directories...)
	sort.Strings(directories)
	var patterns strings.Builder
	for _, directory := range directories {
		if !safeGitPath(directory) {
			return nil, fmt.Errorf("unsafe sparse skill directory %q", directory)
		}
		patterns.WriteByte('/')
		for _, r := range directory {
			if strings.ContainsRune(`\*?[] `, r) {
				patterns.WriteByte('\\')
			}
			patterns.WriteRune(r)
		}
		patterns.WriteString("/\n")
	}
	return []byte(patterns.String()), nil
}

func safeGitPath(value string) bool {
	if value == "" || strings.ContainsAny(value, "\x00\n") || strings.HasPrefix(value, "/") || path.Clean(value) != value {
		return false
	}
	if os.PathSeparator == '\\' && strings.ContainsRune(value, '\\') {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == "." || component == ".." {
			return false
		}
	}
	return true
}

func validateMaterializedTree(repositoryDir, directory string) error {
	repositoryRoot, err := filepath.Abs(repositoryDir)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(filepath.Join(repositoryRoot, filepath.FromSlash(directory)))
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(repositoryRoot, root)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("materialized skill path %q escapes repository", directory)
	}
	realRepository, err := filepath.EvalSymlinks(repositoryRoot)
	if err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	relative, err = filepath.Rel(realRepository, realRoot)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("materialized skill path %q escapes repository through a symbolic link", directory)
	}
	return filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("materialized skill contains symbolic link %s", filePath)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("materialized skill contains non-regular file %s", filePath)
		}
		return nil
	})
}
