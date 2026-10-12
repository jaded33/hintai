package config

import (
	"os"
	"testing"
)

func TestCloudEnvironmentKeyOverridesConfig(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "environment-key")
	cfg := Config{Provider: "openai", Model: "gpt-test", APIKey: "yaml-key"}
	cfg.APIKey = firstEnv(envForProvider(cfg.Provider), cfg.APIKey)
	if cfg.APIKey != "environment-key" {
		t.Fatalf("got %q", cfg.APIKey)
	}
}
func TestLegacyGoogleKeyRemainsSupported(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "legacy-google-key")
	if got := firstEnv(envForProvider("gemini"), ""); got != "legacy-google-key" {
		t.Fatalf("got %q", got)
	}
}
func TestConfigValidation(t *testing.T) {
	if err := (&Config{Provider: "ollama", BaseURL: "http://localhost", DefaultModel: "model"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (&Config{Provider: "ollama", BaseURL: "http://localhost", DefaultModel: "model", Shell: "tcsh"}).Validate(); err == nil {
		t.Fatal("expected unsupported shell")
	}
	_ = os.Getenv("HOME")
}
