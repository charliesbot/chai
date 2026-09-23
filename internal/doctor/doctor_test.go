package doctor

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charliesbot/chai/internal/config"
)

func TestRunWithHomeReportsUnmanagedSkillsDeterministicallyWithoutWriting(t *testing.T) {
	home := t.TempDir()
	cfg := &config.Config{Platforms: []string{"cursor", "claude"}}
	for _, path := range []string{
		filepath.Join(home, ".cursor", "skills", "zeta"),
		filepath.Join(home, ".cursor", "skills", "alpha"),
		filepath.Join(home, ".claude", "skills", "bravo"),
	} {
		writeDir(t, path)
	}

	before := treeState(t, home)
	output, err := captureOutput(t, func() error { return RunWithHome(context.Background(), cfg, home) })
	if err != nil {
		t.Fatalf("RunWithHome: %v", err)
	}
	if after := treeState(t, home); after != before {
		t.Fatalf("doctor modified filesystem:\nbefore=%s\nafter=%s", before, after)
	}
	for _, want := range []string{
		"INFO  Unmanaged skill: bravo", "Platform: Claude", filepath.Join(home, ".claude", "skills", "bravo"),
		"INFO  Unmanaged skill: alpha", "Platform: Cursor", filepath.Join(home, ".cursor", "skills", "alpha"),
		"INFO  Unmanaged skill: zeta", "Not tracked by Chai; left untouched.", "No action required unless you want Chai to manage it.",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Index(output, "bravo") > strings.Index(output, "alpha") || strings.Index(output, "alpha") > strings.Index(output, "zeta") {
		t.Errorf("output is not deterministic:\n%s", output)
	}
}

func TestRunWithHomeReportsConfiguredUnmanagedFilesAndSymlinksAsErrors(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "sources", "chosen")
	writeSkill(t, source, "chosen")
	file := filepath.Join(home, ".cursor", "skills", "chosen")
	writeDir(t, filepath.Dir(file))
	if err := os.WriteFile(file, []byte("user file"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Platforms: []string{"cursor"}, Skills: config.Skills{Local: []string{source}}}
	output, err := captureOutput(t, func() error { return RunWithHome(context.Background(), cfg, home) })
	if err == nil || !strings.Contains(output, "ERROR Unmanaged destination blocks configured skill: chosen") {
		t.Fatalf("file collision: output=%q err=%v", output, err)
	}

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(home, "user-managed")
	writeDir(t, real)
	if err := os.Symlink(real, file); err != nil {
		t.Fatal(err)
	}
	output, err = captureOutput(t, func() error { return RunWithHome(context.Background(), cfg, home) })
	if err == nil || !strings.Contains(output, "ERROR Unmanaged destination blocks configured skill: chosen") {
		t.Fatalf("symlink collision: output=%q err=%v", output, err)
	}
}

func TestRunWithHomeFailsWhenConfiguredDestinationCannotBeInspected(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "sources", "chosen")
	writeSkill(t, source, "chosen")
	dest := filepath.Join(home, ".cursor", "skills", "chosen")
	writeDir(t, filepath.Dir(dest))
	if err := os.Symlink(dest, dest); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Platforms: []string{"cursor"}, Skills: config.Skills{Local: []string{source}}}

	_, err := captureOutput(t, func() error { return RunWithHome(context.Background(), cfg, home) })
	if err == nil || !strings.Contains(err.Error(), "checking skill destination") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunWithHomeReportsConfiguredUnmanagedDestinationAsError(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "sources", "chosen")
	writeSkill(t, source, "chosen")
	blocked := filepath.Join(home, ".cursor", "skills", "chosen")
	writeDir(t, blocked)
	cfg := &config.Config{Platforms: []string{"cursor"}, Skills: config.Skills{Local: []string{source}}}

	output, err := captureOutput(t, func() error { return RunWithHome(context.Background(), cfg, home) })
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(output, "ERROR Unmanaged destination blocks configured skill: chosen") || !strings.Contains(output, blocked) {
		t.Fatalf("output = %q", output)
	}
	if !strings.Contains(err.Error(), "unmanaged skill destinations block sync") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunWithHomeSkipsMissingPlatformSkillDirectories(t *testing.T) {
	home := t.TempDir()
	output, err := captureOutput(t, func() error {
		return RunWithHome(context.Background(), &config.Config{Platforms: []string{"cursor"}}, home)
	})
	if err != nil {
		t.Fatalf("RunWithHome: %v", err)
	}
	if !strings.Contains(output, "no unmanaged skill destinations found") {
		t.Fatalf("output = %q", output)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor")); !os.IsNotExist(err) {
		t.Fatalf("doctor created platform directory: %v", err)
	}
}

func TestRunWithHomeReportsInvalidHashDB(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".chai", "hashes.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := captureOutput(t, func() error { return RunWithHome(context.Background(), &config.Config{}, home) })
	if err == nil || !strings.Contains(err.Error(), "parsing hash DB") {
		t.Fatalf("error = %v", err)
	}
}

func writeDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func writeSkill(t *testing.T, path, name string) {
	t.Helper()
	writeDir(t, path)
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func captureOutput(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	err = fn()
	_ = w.Close()
	os.Stdout = old
	data, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(data), err
}

func treeState(t *testing.T, root string) string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		entry := fmt.Sprintf("%s %s", rel, info.Mode())
		switch {
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry += fmt.Sprintf(" %q", data)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry += fmt.Sprintf(" -> %q", target)
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(entries, "\n")
}
