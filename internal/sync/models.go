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
		models = defaultNoestelarCustomModels()
	}
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

func defaultNoestelarCustomModels() []config.CustomModel {
	models := []string{
		"ollama/gpt-oss:120b",
		"ollama/kimi-k2.5",
		"ollama/glm-5",
		"ollama/minimax-m2.5",
		"ollama/glm-4.7-flash",
		"ollama/qwen3.5",
		"gh/gpt-3.5-turbo",
		"gh/gpt-4",
		"gh/gpt-4o",
		"gh/gpt-4o-mini",
		"gh/gpt-4.1",
		"gh/gpt-5-mini",
		"gh/gpt-5.2",
		"gh/gpt-5.2-codex",
		"gh/gpt-5.3-codex",
		"gh/gpt-5.4",
		"gh/gpt-5.4-mini",
		"gh/claude-haiku-4.5",
		"gh/claude-opus-4.5",
		"gh/claude-sonnet-4",
		"gh/claude-sonnet-4.5",
		"gh/claude-sonnet-4.6",
		"gh/claude-opus-4.6",
		"gh/claude-opus-4.7",
		"gh/gemini-2.5-pro",
		"gh/gemini-3-flash-preview",
		"gh/gemini-3.1-pro-preview",
		"gh/grok-code-fast-1",
		"gh/oswe-vscode-prime",
		"gh/goldeneye-free-auto",
		"cc/claude-opus-4-7",
		"cc/claude-opus-4-6",
		"cc/claude-sonnet-4-6",
		"cc/claude-opus-4-5-20251101",
		"cc/claude-sonnet-4-5-20250929",
		"cc/claude-haiku-4-5-20251001",
		"cx/gpt-5.5",
		"cx/gpt-5.4",
		"cx/gpt-5.3-codex",
		"cx/gpt-5.3-codex-xhigh",
		"cx/gpt-5.3-codex-high",
		"cx/gpt-5.3-codex-low",
		"cx/gpt-5.3-codex-none",
		"cx/gpt-5.3-codex-spark",
		"cx/gpt-5.1-codex-mini",
		"cx/gpt-5.1-codex-mini-high",
		"cx/gpt-5.2-codex",
		"cx/gpt-5.2",
		"cx/gpt-5.1-codex-max",
		"cx/gpt-5.1-codex",
		"cx/gpt-5.1",
		"cx/gpt-5-codex",
		"cx/gpt-5-codex-mini",
		"cx/gpt-5.4-image",
		"cx/gpt-5.3-image",
		"cx/gpt-5.2-image",
		"gc/gemini-3-flash-preview",
		"gc/gemini-3-pro-preview",
	}
	out := make([]config.CustomModel, len(models))
	for i, model := range models {
		out[i] = config.CustomModel{
			Model:           model,
			DisplayName:     model + " [Noestelar]",
			BaseURL:         "https://inference.noestelar.com/v1",
			APIKey:          "${NOESTELAR_INFERENCE_API_KEY}",
			Provider:        "generic-chat-completion-api",
			MaxOutputTokens: 16384,
		}
	}
	return out
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
