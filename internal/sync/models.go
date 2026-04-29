package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/ui"
)

func syncDroidCustomModels(cfg *config.Config, home string, dryRun bool) error {
	models := cfg.Droid.CustomModels
	if len(models) == 0 {
		return nil
	}
	if dryRun {
		fmt.Println(ui.Label.Render("droid custom models"))
		preview := map[string]any{"customModels": models}
		data, err := json.MarshalIndent(preview, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling Droid custom model preview: %w", err)
		}
		fmt.Println(ui.JSONBlock.Render(string(data)))
		fmt.Println()
		return nil
	}
	path := filepath.Join(home, ".factory", "settings.json")
	if err := mergeDroidCustomModels(path, models); err != nil {
		return err
	}
	names := make([]string, len(models))
	for i, m := range models {
		names[i] = m.Model
	}
	fmt.Println(ui.Box("droid custom models", len(names), []ui.PlatformStatus{{Name: "Droid", State: ui.PlatformOK}}, names))
	return nil
}

func mergeDroidCustomModels(path string, models []config.CustomModel) error {
	settings := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	settings["customModels"] = models
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", path, err)
	}
	out = append(out, '\n')
	if err := atomicWrite(path, out); err != nil {
		return fmt.Errorf("writing Droid custom models to %s: %w", path, err)
	}
	return nil
}
