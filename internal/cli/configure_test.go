package cli

import (
	"testing"

	"github.com/jaded33/hintai/internal/config"
)

func TestSwitchProviderDoesNotCarryPreviousCredentialsOrModels(t *testing.T) {
	cfg := config.Config{
		Provider:     "anthropic",
		BaseURL:      "https://api.anthropic.com/v1",
		APIKey:       "anthropic-key",
		Model:        "claude-model",
		DefaultModel: "local-model",
	}
	switchProvider(&cfg, "openai")
	if cfg.APIKey != "" || cfg.Model != "" || cfg.DefaultModel != "" {
		t.Fatalf("provider-specific credentials or model carried over: %+v", cfg)
	}
	if cfg.BaseURL != config.DefaultBaseURL("openai") {
		t.Fatalf("base URL = %q", cfg.BaseURL)
	}
}

func TestSwitchingToSameProviderPreservesConfiguration(t *testing.T) {
	cfg := config.Config{Provider: "openai", APIKey: "key", BaseURL: "https://gateway.example/v1", Model: "model"}
	switchProvider(&cfg, "openai")
	if cfg.APIKey != "key" || cfg.BaseURL != "https://gateway.example/v1" || cfg.Model != "model" {
		t.Fatalf("same-provider values changed: %+v", cfg)
	}
}
