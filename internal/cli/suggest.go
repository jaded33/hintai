package cli

import (
	"context"
	"fmt"

	"github.com/jaded33/hintai/internal/config"
	"github.com/jaded33/hintai/internal/history"
	"github.com/jaded33/hintai/internal/llm"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const defaultSuggestHistoryContextLimit = 100

var suggestCmd = &cobra.Command{
	Use:   "suggest \"<task description>\"",
	Short: "Suggest shell commands using AI",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		q := args[0]
		if q == "" {
			return fmt.Errorf("task description cannot be empty")
		}
		limit, _ := cmd.Flags().GetInt("limit")
		noHistory, _ := cmd.Flags().GetBool("no-history-context")
		shell, _ := cmd.Flags().GetString("shell")
		path, _ := cmd.Flags().GetString("history-file")
		result, err := runSuggestCore(cmd.Context(), logger, q, limit, noHistory, shell, path)
		if err != nil {
			return err
		}
		return printCommandOutput(logger, result, "--- Suggested Commands ---", "No suggestions generated.")
	},
}

func runSuggestCore(ctx context.Context, logger *zap.Logger, query string, limit int, noHistory bool, shell, path string) (string, error) {
	if limit < 0 {
		return "", fmt.Errorf("history limit cannot be negative")
	}
	cfg, err := config.LoadConfig(logger)
	if err != nil {
		return "", err
	}
	entries := []history.HistoryEntry{}
	if !noHistory {
		if shell == "" {
			shell = cfg.Shell
		}
		if path == "" {
			path = cfg.HistoryFile
		}
		reader, err := history.NewReader(shell, path, logger)
		if err != nil {
			return "", err
		}
		entries, err = reader.ReadHistory(limit)
		if err != nil {
			return "", err
		}
	}
	client, err := llm.NewClientFromConfig(ctx, logger, cfg)
	if err != nil {
		return "", err
	}
	defer client.Close()
	result, err := client.SuggestCommands(ctx, query, entries)
	if err != nil {
		return "", err
	}
	if result == "" {
		return "(AI could not suggest a command for this task)", nil
	}
	return result, nil
}

func init() {
	rootCmd.AddCommand(suggestCmd)
	suggestCmd.Flags().IntP("limit", "n", defaultSuggestHistoryContextLimit, "Number of recent history entries (0 means all)")
	suggestCmd.Flags().Bool("no-history-context", false, "Do not send shell history as suggestion context")
	suggestCmd.Flags().String("shell", "", "History shell: zsh, bash, or fish (auto-detected by default)")
	suggestCmd.Flags().String("history-file", "", "Use this history file instead of the shell default")
}
