package deps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const depsDir = ".chai/deps"

// Action describes what happened to a dep.
type Action string

const (
	ActionCloned  Action = "cloned"
	ActionPulled  Action = "pulled"
	ActionCurrent Action = "up to date"
)

// Result holds the outcome of syncing a single dep.
type Result struct {
	Name   string
	URL    string
	Action Action
	Err    error
}

// SyncOne clones or pulls a single dependency and returns the result.
func SyncOne(name, url, home string) Result {
	base := filepath.Join(home, depsDir)
	dest := filepath.Join(base, name)

	if err := os.MkdirAll(base, 0755); err != nil {
		return Result{Name: name, URL: url, Err: fmt.Errorf("creating deps directory: %w", err)}
	}

	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		return pullDep(name, url, dest)
	}
	return cloneDep(name, url, dest)
}

func cloneDep(name, url, dest string) Result {
	cmd := exec.Command("git", "clone", "--quiet", url, dest)
	if out, err := cmd.CombinedOutput(); err != nil {
		return Result{Name: name, URL: url, Err: fmt.Errorf("cloning: %s", string(out))}
	}
	return Result{Name: name, URL: url, Action: ActionCloned}
}

func pullDep(name, url, dest string) Result {
	cmd := exec.Command("git", "pull", "--quiet")
	cmd.Dir = dest
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{Name: name, URL: url, Err: fmt.Errorf("pulling: %s", string(out))}
	}
	output := string(out)
	if output == "" || output == "Already up to date.\n" {
		return Result{Name: name, URL: url, Action: ActionCurrent}
	}
	return Result{Name: name, URL: url, Action: ActionPulled}
}

// SyncWithHome clones or pulls all dependencies (non-interactive, for tests).
func SyncWithHome(depMap map[string]string, home string) error {
	if len(depMap) == 0 {
		return nil
	}

	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is required but not found in PATH")
	}

	for name, url := range depMap {
		r := SyncOne(name, url, home)
		if r.Err != nil {
			return fmt.Errorf("dep %q: %w", name, r.Err)
		}
	}

	return nil
}
