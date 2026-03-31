package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	OpenRouterKey   string `json:"openrouter_key"`
	OpenRouterModel string `json:"openrouter_model"`
	LocalScrubber   struct {
		Enabled    bool   `json:"enabled"`
		URL        string `json:"url"`
		BinaryPath string `json:"binary_path"`
		ModelPath  string `json:"model_path"`
	} `json:"local_scrubber"`
}

func Load() (*Config, error) {
	config := &Config{
		OpenRouterModel: "meta-llama/llama-3.2-3b-instruct:free",
	}
	config.LocalScrubber.URL = "http://localhost:8080/v1/chat/completions"

	// Try loading from configs/armage.json or fallback to armage.json
	configPaths := []string{"configs/armage.json", "armage.json"}
	for _, path := range configPaths {
		configData, err := os.ReadFile(path)
		if err == nil {
			json.Unmarshal(configData, config)
			break
		}
	}

	// Environment variable overrides
	if apiKey := os.Getenv("OPENROUTER_API_KEY"); apiKey != "" {
		config.OpenRouterKey = apiKey
	}
	if model := os.Getenv("OPENROUTER_MODEL"); model != "" {
		config.OpenRouterModel = model
	}
	if scrubberURL := os.Getenv("LOCAL_SCRUBBER_URL"); scrubberURL != "" {
		config.LocalScrubber.URL = scrubberURL
		config.LocalScrubber.Enabled = true
	}

	return config, nil
}
