package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const configDir = ".config/hintai"
const configFileName = "config.yaml"

type Config struct {
	Provider         string `yaml:"provider"`
	BaseURL          string `yaml:"base_url,omitempty"`
	DefaultModel     string `yaml:"default_model,omitempty"`
	ReuseLoadedModel bool   `yaml:"reuse_loaded_model"`
	DisableThinking  bool   `yaml:"disable_thinking"`
	APIKey           string `yaml:"api_key,omitempty"`
	Model            string `yaml:"model,omitempty"`
	Shell            string `yaml:"shell,omitempty"`
	HistoryFile      string `yaml:"history_file,omitempty"`
}

func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDir, configFileName), nil
}
func LoadConfig(logger *zap.Logger) (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, fmt.Errorf("find config path: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("configuration not found at %s; run 'hintai configure'", path)
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.APIKey = firstEnv(envForProvider(cfg.Provider), cfg.APIKey)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	logger.Debug("Configuration loaded", zap.String("provider", cfg.Provider))
	return &cfg, nil
}
func envForProvider(provider string) string {
	switch provider {
	case "openai":
		return "OPENAI_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "gemini":
		return "GEMINI_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	case "grok":
		return "XAI_API_KEY"
	}
	return ""
}
func firstEnv(name, fallback string) string {
	if name != "" && os.Getenv(name) != "" {
		return os.Getenv(name)
	}
	if name == "GEMINI_API_KEY" && os.Getenv("GOOGLE_API_KEY") != "" {
		return os.Getenv("GOOGLE_API_KEY")
	}
	return fallback
}
func (c *Config) Validate() error {
	switch c.Provider {
	case "lmstudio":
		if c.BaseURL == "" {
			return fmt.Errorf("base_url is required for lmstudio")
		}
		if c.DefaultModel == "" && !c.ReuseLoadedModel {
			return fmt.Errorf("set default_model or enable reuse_loaded_model")
		}
	case "ollama":
		if c.BaseURL == "" {
			return fmt.Errorf("base_url is required for ollama")
		}
		if c.DefaultModel == "" {
			return fmt.Errorf("default_model is required for ollama")
		}
	case "openai", "anthropic", "gemini", "openrouter", "grok":
		if c.APIKey == "" {
			return fmt.Errorf("set %s or api_key in config", envForProvider(c.Provider))
		}
		if c.Model == "" {
			return fmt.Errorf("model is required for %s", c.Provider)
		}
	default:
		return fmt.Errorf("unknown provider %q", c.Provider)
	}
	if c.Shell != "" && c.Shell != "zsh" && c.Shell != "bash" && c.Shell != "fish" {
		return fmt.Errorf("unsupported shell %q", c.Shell)
	}
	return nil
}
func (c *Config) IsLocal() bool { return c.Provider == "lmstudio" || c.Provider == "ollama" }
