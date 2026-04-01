package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	// 1. Test Default Values
	cfg, _ := Load()
	if cfg.OpenRouterModel != "meta-llama/llama-3.2-3b-instruct:free" {
		t.Errorf("Expected default model, got %s", cfg.OpenRouterModel)
	}

	// 2. Test Environment Overrides
	os.Setenv("OPENROUTER_API_KEY", "test-key-env")
	os.Setenv("LOCAL_SCRUBBER_URL", "http://env-url")
	defer os.Unsetenv("OPENROUTER_API_KEY")
	defer os.Unsetenv("LOCAL_SCRUBBER_URL")

	cfg, _ = Load()
	if cfg.OpenRouterKey != "test-key-env" {
		t.Errorf("Expected env key, got %s", cfg.OpenRouterKey)
	}
	if cfg.LocalScrubber.URL != "http://env-url" || !cfg.LocalScrubber.Enabled {
		t.Errorf("Expected env URL and Enabled=true, got %s, %v", cfg.LocalScrubber.URL, cfg.LocalScrubber.Enabled)
	}

	// 3. Test File Loading
	os.MkdirAll("configs", 0755)
	content := `{"openrouter_key": "file-key", "openrouter_model": "file-model"}`
	os.WriteFile("configs/armage.json", []byte(content), 0644)
	// We don't unset the env vars, so they should still take precedence if we follow Load logic correctly.
	// But let's unset them to test file priority over defaults.
	os.Unsetenv("OPENROUTER_API_KEY")
	
	cfg, _ = Load()
	if cfg.OpenRouterKey != "file-key" {
		t.Errorf("Expected file key, got %s", cfg.OpenRouterKey)
	}
	if cfg.OpenRouterModel != "file-model" {
		t.Errorf("Expected file model, got %s", cfg.OpenRouterModel)
	}
	
	// Cleanup
	os.Remove("configs/armage.json")
}
