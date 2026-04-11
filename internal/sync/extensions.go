package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/charliesbot/chai/internal/ui"
)

// syncGeminiExtensions ensures Gemini extensions are installed.
func syncGeminiExtensions(extensions map[string]string, home string, dryRun bool) error {
	if len(extensions) == 0 {
		return nil
	}

	extDir := filepath.Join(home, ".gemini", "extensions")

	var toInstall []string
	var alreadyInstalled []string

	for name, url := range extensions {
		installed := filepath.Join(extDir, name)
		if _, err := os.Stat(installed); err == nil {
			alreadyInstalled = append(alreadyInstalled, name)
		} else {
			if dryRun {
				toInstall = append(toInstall, name)
			} else {
				fmt.Printf("  installing %s ...\n", ui.Bold.Render(name))
				cmd := exec.Command("gemini", "extensions", "install", url)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("installing gemini extension %q: %w", name, err)
				}
				toInstall = append(toInstall, name)
			}
		}
	}

	if dryRun {
		fmt.Println(ui.Label.Render("gemini extensions"))
		for _, name := range alreadyInstalled {
			fmt.Printf("  %s %s %s\n", ui.Check(), ui.Bold.Render(name), ui.Muted.Render("installed"))
		}
		for _, name := range toInstall {
			fmt.Printf("  %s %s %s\n", ui.Arrow(), ui.Bold.Render(name), ui.Muted.Render("will install"))
		}
		fmt.Println()
		return nil
	}

	// Build display lists
	allNames := make([]string, 0, len(extensions))
	for name := range extensions {
		allNames = append(allNames, name)
	}

	fmt.Println(ui.Box("gemini extensions", len(allNames), ui.PlatformNA, ui.PlatformOK, allNames))

	return nil
}
