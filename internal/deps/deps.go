package deps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const depsDir = ".chai/deps"

// Sync clones or pulls all dependencies.
func Sync(depMap map[string]string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	return SyncWithHome(depMap, home)
}

// SyncWithHome clones or pulls all dependencies using the given home directory.
func SyncWithHome(depMap map[string]string, home string) error {
	if len(depMap) == 0 {
		return nil
	}

	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is required but not found in PATH")
	}

	base := filepath.Join(home, depsDir)
	if err := os.MkdirAll(base, 0755); err != nil {
		return fmt.Errorf("creating deps directory: %w", err)
	}

	for name, url := range depMap {
		dest := filepath.Join(base, name)
		if err := syncOne(name, url, dest); err != nil {
			return err
		}
	}

	return nil
}

func syncOne(name, url, dest string) error {
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		return pull(name, dest)
	}
	return clone(name, url, dest)
}

func clone(name, url, dest string) error {
	fmt.Printf("cloning %s ...\n", name)
	cmd := exec.Command("git", "clone", url, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cloning dep %q from %s: %w", name, url, err)
	}
	return nil
}

func pull(name, dest string) error {
	fmt.Printf("pulling %s ...\n", name)
	cmd := exec.Command("git", "pull")
	cmd.Dir = dest
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pulling dep %q in %s: %w", name, dest, err)
	}
	return nil
}
