package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var version = "dev"
var (
	logger    *zap.Logger
	debugMode bool
	rootCmd   = &cobra.Command{
		Use:          "hintai",
		Version:      version,
		Short:        "Find and suggest shell commands using AI.",
		Long:         "HintAI searches shell history and suggests commands using a configured local or cloud language model.",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "version" {
				return nil
			}
			var cfg zap.Config
			if debugMode {
				cfg = zap.NewDevelopmentConfig()
			} else {
				cfg = zap.NewProductionConfig()
				cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
			}
			var err error
			logger, err = cfg.Build()
			if err != nil {
				return fmt.Errorf("initialize logger: %w", err)
			}
			return nil
		},
	}
)

func Execute() error { return rootCmd.Execute() }
func init() {
	rootCmd.AddCommand(&cobra.Command{Use: "version", Short: "Print version", Run: func(cmd *cobra.Command, args []string) { fmt.Fprintln(cmd.OutOrStdout(), version) }})
	rootCmd.PersistentFlags().BoolVarP(&debugMode, "debug", "d", false, "Enable debug logging")
}
