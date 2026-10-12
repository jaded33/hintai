package llm

import (
	"context"
	"github.com/jaded33/hintai/internal/history"
)

type LLMClient interface {
	FindHistoryEntries(context.Context, string, []history.HistoryEntry) (string, error)
	SuggestCommands(context.Context, string, []history.HistoryEntry) (string, error)
	Close() error
}
