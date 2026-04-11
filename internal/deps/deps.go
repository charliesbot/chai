package deps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/charliesbot/chai/internal/config"
)

const depsDir = ".chai/deps"

// Action describes what happened to a dep.
type Action string

const (
	ActionCloned  Action = "cloned"
	ActionPulled  Action = "pulled"
	ActionCurrent Action = "up to date"
	ActionBuilt   Action = "built"
)

// Result holds the outcome of syncing a single dep.
type Result struct {
	Name   string
	URL    string
	Action Action
	Built  bool
	Err    error
}

// SyncOne clones or pulls a single dependency and returns the result.
func SyncOne(name string, dep config.Dep, home string) Result {
	base := filepath.Join(home, depsDir)
	dest := filepath.Join(base, name)

	if err := os.MkdirAll(base, 0755); err != nil {
		return Result{Name: name, URL: dep.URL, Err: fmt.Errorf("creating deps directory: %w", err)}
	}

	var result Result
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		result = pullDep(name, dep.URL, dest)
	} else {
		result = cloneDep(name, dep.URL, dest)
	}

	if result.Err != nil {
		return result
	}

	// Run build on first clone only
	if dep.Build != "" && result.Action == ActionCloned {
		if err := runBuild(dep.Build, dest); err != nil {
			result.Err = fmt.Errorf("build failed: %w", err)
			return result
		}
		result.Built = true
	}

	return result
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

func runBuild(buildCmd, dir string) error {
	cmd := exec.Command("sh", "-c", buildCmd)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// SyncWithHome clones or pulls all dependencies (non-interactive, for tests).
func SyncWithHome(depMap map[string]config.Dep, home string) error {
	if len(depMap) == 0 {
		return nil
	}

	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is required but not found in PATH")
	}

	for name, dep := range depMap {
		r := SyncOne(name, dep, home)
		if r.Err != nil {
			return fmt.Errorf("dep %q: %w", name, r.Err)
		}
	}

	return nil
}
