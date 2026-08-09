package githubskill

import (
	"context"
	"strings"
	"testing"
)

func TestParseGitVersion(t *testing.T) {
	tests := []struct {
		raw  string
		want Version
	}{
		{"git version 2.37.0", Version{Major: 2, Minor: 37, Patch: 0}},
		{"git version 2.50.1 (Apple Git-155)", Version{Major: 2, Minor: 50, Patch: 1}},
		{"git version 2.39.3.windows.1", Version{Major: 2, Minor: 39, Patch: 3}},
		{"git version 3.0", Version{Major: 3, Minor: 0, Patch: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseGitVersion(tt.raw)
			if err != nil {
				t.Fatalf("ParseGitVersion: %v", err)
			}
			if got != tt.want {
				t.Fatalf("version = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRequireGitVersion(t *testing.T) {
	if err := requireGitVersion(Version{Major: 2, Minor: 37}); err != nil {
		t.Fatalf("2.37 should be supported: %v", err)
	}
	err := requireGitVersion(Version{Major: 2, Minor: 36, Patch: 6})
	if err == nil || !strings.Contains(err.Error(), "detected 2.36.6") {
		t.Fatalf("outdated error = %v", err)
	}
}

func TestCheckGit_Missing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := CheckGit(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not found in PATH") {
		t.Fatalf("missing Git error = %v", err)
	}
}

func TestParseGitVersion_RejectsUnknownOutput(t *testing.T) {
	_, err := ParseGitVersion("git custom build")
	if err == nil || !strings.Contains(err.Error(), "git custom build") {
		t.Fatalf("parse error = %v", err)
	}
}
