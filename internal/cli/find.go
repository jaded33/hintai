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

const defaultFindHistoryLimit = 300

var findCmd = &cobra.Command{
	Use:   "find \"<natural language query>\"",
	Short: "Find commands in shell history using a natural language query",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		q := args[0]
		if q == "" {
			return fmt.Errorf("query cannot be empty")
		}
		limit, _ := cmd.Flags().GetInt("limit")
		shell, _ := cmd.Flags().GetString("shell")
		path, _ := cmd.Flags().GetString("history-file")
		result, err := runFind(cmd.Context(), logger, q, limit, shell, path)
		if err != nil {
			return err
		}
		return printCommandOutput(logger, result, "--- Found Commands ---", "No matching history entries found.")
	},
}

func runFind(ctx context.Context, logger *zap.Logger, query string, limit int, shell, path string) (string, error) {
	if limit < 0 {
		return "", fmt.Errorf("history limit cannot be negative")
	}
	cfg, err := config.LoadConfig(logger)
	if err != nil {
		return "", err
	}
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
	entries, err := reader.ReadHistory(limit)
	if err != nil {
		return "", err
	}
	client, err := llm.NewClientFromConfig(ctx, logger, cfg)
	if err != nil {
		return "", err
	}
	defer client.Close()
	result, err := client.FindHistoryEntries(ctx, query, entries)
	if err != nil {
		return "", err
	}
	if result == "" {
		return "(No relevant commands found)", nil
	}
	return result, nil
}

func init() {
	rootCmd.AddCommand(findCmd)
	findCmd.Flags().IntP("limit", "n", defaultFindHistoryLimit, "Number of recent history entries (0 means all)")
	findCmd.Flags().String("shell", "", "History shell: zsh, bash, or fish (auto-detected by default)")
	findCmd.Flags().String("history-file", "", "Use this history file instead of the shell default")
}
