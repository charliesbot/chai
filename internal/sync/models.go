package sync

import (
	"encoding/json"
	"fmt"
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
	return replaceJSONKey(path, "customModels", models)
}
