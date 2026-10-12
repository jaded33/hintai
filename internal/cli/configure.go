package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jaded33/hintai/internal/config"
	"github.com/jaded33/hintai/internal/history"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Set up an LLM provider and shell history",
	RunE:  func(cmd *cobra.Command, args []string) error { return configure(os.Stdin, os.Stdout) },
}

func configure(in *os.File, out *os.File) error {
	path, err := config.Path()
	if err != nil {
		return err
	}
	cfg := config.Config{Provider: "lmstudio", BaseURL: config.DefaultBaseURL("lmstudio"), ReuseLoadedModel: true, DisableThinking: true, Shell: history.DetectShell()}
	if data, e := os.ReadFile(path); e == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parse existing config: %w", err)
		}
	} else if !os.IsNotExist(e) {
		return fmt.Errorf("read existing config: %w", e)
	}
	reader := bufio.NewReader(in)
	ask := func(label, def string) (string, error) {
		if def != "" {
			fmt.Fprintf(out, "%s [%s]: ", label, def)
		} else {
			fmt.Fprintf(out, "%s: ", label)
		}
		line, e := reader.ReadString('\n')
		if e != nil && len(line) == 0 {
			return "", e
		}
		line = strings.TrimSpace(line)
		if line == "" {
			line = def
		}
		return line, nil
	}
	fmt.Fprintln(out, "HintAI setup. find sends history entries to your provider. suggest sends history context unless disabled.")
	provider, e := ask("Provider (lmstudio, ollama, openai, anthropic, gemini, openrouter, grok)", cfg.Provider)
	if e != nil {
		return e
	}
	provider = strings.ToLower(provider)
	switchProvider(&cfg, provider)
	base, e := ask("Provider base URL", cfg.BaseURL)
	if e != nil {
		return e
	}
	cfg.BaseURL = base
	providerType := "cloud"
	if cfg.IsLocal() {
		providerType = "local"
	}
	fmt.Fprintf(out, "Provider type: %s. History context will be sent to %s at %s.\n", providerType, cfg.Provider, cfg.BaseURL)
	if cfg.IsLocal() {
		model, e := ask("Default model", cfg.DefaultModel)
		if e != nil {
			return e
		}
		cfg.DefaultModel = model
	} else {
		model, e := ask("Model", cfg.Model)
		if e != nil {
			return e
		}
		cfg.Model = model
	}
	if envName := config.APIKeyEnvironment(cfg.Provider); envName != "" && os.Getenv(envName) == "" {
		if cfg.Provider != "ollama" {
			fmt.Fprintf(out, "%s (leave empty to set later through the environment): ", envName)
			fmt.Fprint(out, "[hidden] ")
			key, e := term.ReadPassword(int(in.Fd()))
			fmt.Fprintln(out)
			if e == nil && len(strings.TrimSpace(string(key))) > 0 {
				cfg.APIKey = strings.TrimSpace(string(key))
			}
		}
	}
	shell, e := ask("History shell (zsh, bash, fish)", cfg.Shell)
	if e != nil {
		return e
	}
	cfg.Shell = shell
	historyPath, e := ask("History file (blank uses shell default)", cfg.HistoryFile)
	if e != nil {
		return e
	}
	cfg.HistoryFile = historyPath
	if e := cfg.ValidateForConfigure(); e != nil {
		return e
	}
	data, e := yaml.Marshal(&cfg)
	if e != nil {
		return e
	}
	dir := filepath.Dir(path)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	tmp, e := os.CreateTemp(dir, ".config-*.yaml")
	if e != nil {
		return e
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if e = tmp.Chmod(0600); e != nil {
		tmp.Close()
		return e
	}
	if _, e = tmp.Write(data); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Sync(); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Close(); e != nil {
		return e
	}
	if e = os.Rename(tmpName, path); e != nil {
		return e
	}
	fmt.Fprintln(out, "Configuration saved to", path)
	return nil
}

func switchProvider(cfg *config.Config, provider string) {
	if provider != cfg.Provider {
		cfg.APIKey = ""
		cfg.BaseURL = config.DefaultBaseURL(provider)
		cfg.Model = ""
		cfg.DefaultModel = ""
	}
	cfg.Provider = provider
}

func init() { rootCmd.AddCommand(configureCmd) }
