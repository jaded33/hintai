package config

import (
	"os"
	"path/filepath"
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

func TestLMStudioTokenComesFromEnvironment(t *testing.T) {
	t.Setenv("LM_API_TOKEN", "lm-token")
	if got := firstEnv(envForProvider("lmstudio"), "yaml-token"); got != "lm-token" {
		t.Fatalf("got %q", got)
	}
}

func TestCloudConfigDefaultsAndCustomBaseURL(t *testing.T) {
	cfg := Config{Provider: "openai", APIKey: "key", Model: "model"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != DefaultBaseURL("openai") {
		t.Fatalf("default base URL = %q", cfg.BaseURL)
	}
	cfg.BaseURL = "https://gateway.example/v1/"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://gateway.example/v1" {
		t.Fatalf("custom base URL was not normalized: %q", cfg.BaseURL)
	}
}

func TestConfigureAllowsCloudKeyToBeSetLater(t *testing.T) {
	cfg := Config{Provider: "gemini", Model: "model"}
	if err := cfg.ValidateForConfigure(); err != nil {
		t.Fatalf("setup should allow entering the key later: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("runtime config must require an API key")
	}
}

func TestConfigPathRespectsXDGConfigHome(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(configHome, "hintai", "config.yaml")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestBaseURLRejectsEmbeddedCredentials(t *testing.T) {
	cfg := Config{Provider: "openai", BaseURL: "https://user:pass@example.test/v1", APIKey: "key", Model: "model"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("embedded credentials should be rejected")
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
