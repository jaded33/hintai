package llm

import (
	"context"
	"fmt"

	"github.com/jaded33/hintai/internal/config"
	"go.uber.org/zap"
)

// NewClientFromConfig creates the appropriate LLMClient based on the config.
func NewClientFromConfig(ctx context.Context, logger *zap.Logger, cfg *config.Config) (LLMClient, error) {
	switch cfg.Provider {
	case "lmstudio":
		return NewLMStudioClient(ctx, logger, cfg.BaseURL, cfg.DefaultModel, cfg.ReuseLoadedModel, cfg.DisableThinking, cfg.APIKey)
	case "ollama":
		return NewOllamaClient(ctx, logger, cfg.BaseURL, cfg.DefaultModel, cfg.DisableThinking)
	case "openai", "anthropic", "gemini", "openrouter", "grok":
		return NewCloudClient(logger, cfg.Provider, cfg.BaseURL, cfg.APIKey, cfg.Model)
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}
}
