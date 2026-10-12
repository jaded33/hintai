package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jaded33/hintai/internal/history"
	"go.uber.org/zap"
)

// OllamaClient implements the LLMClient interface using a local Ollama server.
type OllamaClient struct {
	logger          *zap.Logger
	httpClient      *http.Client
	baseURL         string
	model           string
	disableThinking bool
}

// NewOllamaClient creates a new Ollama client.
func NewOllamaClient(ctx context.Context, logger *zap.Logger, baseURL, model string, disableThinking bool) (*OllamaClient, error) {
	c := &OllamaClient{
		logger:          logger,
		httpClient:      &http.Client{Timeout: 60 * time.Second},
		baseURL:         baseURL,
		model:           model,
		disableThinking: disableThinking,
	}
	logger.Info("Using Ollama", zap.String("model", model))
	return c, nil
}

// Close is a no-op for Ollama.
func (c *OllamaClient) Close() error { return nil }

// FindHistoryEntries implements LLMClient.
func (c *OllamaClient) FindHistoryEntries(ctx context.Context, query string, historyContext []history.HistoryEntry) (string, error) {
	prompt := buildFindPromptText(query, historyContext)
	if prompt == "" {
		return "(No relevant commands found or history was empty)", nil
	}
	result, err := c.chat(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("ollama API call failed (Find): %w", err)
	}
	if result == "" || result == "No relevant commands found." {
		return "(No relevant commands found or AI response was empty)", nil
	}
	return verifiedMatches(result, historyContext), nil
}

// SuggestCommands implements LLMClient.
func (c *OllamaClient) SuggestCommands(ctx context.Context, taskDescription string, historyContext []history.HistoryEntry) (string, error) {
	prompt := buildSuggestPromptText(taskDescription, historyContext)
	result, err := c.chat(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("ollama API call failed (Suggest): %w", err)
	}
	if result == "" || result == "Cannot suggest a command for this task." {
		return "(AI could not suggest a command for this task)", nil
	}
	return result, nil
}

// chat sends a prompt to Ollama's OpenAI-compatible endpoint.
func (c *OllamaClient) chat(ctx context.Context, prompt string) (string, error) {
	reqBody := chatRequest{
		Model:     c.model,
		MaxTokens: 1024,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", nil
	}

	result := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if c.disableThinking {
		result = stripThinkingTags(result)
	}
	return result, nil
}
