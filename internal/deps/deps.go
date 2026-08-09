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
)

// Result holds the outcome of syncing a single dep.
type Result struct {
	Action Action
	Built  bool
	Err    error
}

// SyncOne clones or pulls a single dependency and returns the result.
func SyncOne(name string, dep config.Dep, home string) Result {
	base := filepath.Join(home, depsDir)
	dest := filepath.Join(base, name)

	if err := os.MkdirAll(base, 0755); err != nil {
		return Result{Err: fmt.Errorf("creating deps directory: %w", err)}
	}

	var result Result
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		result = pullDep(dest)
	} else {
		result = cloneDep(dep.URL, dest)
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

func cloneDep(url, dest string) Result {
	cmd := exec.Command("git", "clone", "--quiet", url, dest)
	if out, err := cmd.CombinedOutput(); err != nil {
		return Result{Err: fmt.Errorf("cloning: %s", string(out))}
	}
	return Result{Action: ActionCloned}
}

func pullDep(dest string) Result {
	cmd := exec.Command("git", "pull", "--quiet")
	cmd.Dir = dest
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{Err: fmt.Errorf("pulling: %s", string(out))}
	}
	output := string(out)
	if output == "" || output == "Already up to date.\n" {
		return Result{Action: ActionCurrent}
	}
	return Result{Action: ActionPulled}
}

func runBuild(buildCmd, dir string) error {
	cmd := exec.Command("sh", "-c", buildCmd)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
