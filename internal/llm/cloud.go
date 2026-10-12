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

// CloudClient implements LLMClient for OpenAI-compatible cloud APIs.
type CloudClient struct {
	logger     *zap.Logger
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

var cloudBaseURLs = map[string]string{
	"openai":     "https://api.openai.com/v1",
	"anthropic":  "https://api.anthropic.com/v1",
	"gemini":     "https://generativelanguage.googleapis.com/v1beta/openai",
	"openrouter": "https://openrouter.ai/api/v1",
	"grok":       "https://api.x.ai/v1",
}

// NewCloudClient creates a new cloud LLM client.
func NewCloudClient(ctx context.Context, logger *zap.Logger, provider, apiKey, model string) (*CloudClient, error) {
	baseURL, ok := cloudBaseURLs[provider]
	if !ok {
		return nil, fmt.Errorf("unknown cloud provider: %s", provider)
	}
	logger.Info("Using cloud provider", zap.String("provider", provider), zap.String("model", model))
	return &CloudClient{
		logger:     logger,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
	}, nil
}

// Close is a no-op for cloud clients.
func (c *CloudClient) Close() error { return nil }

// FindHistoryEntries implements LLMClient.
func (c *CloudClient) FindHistoryEntries(ctx context.Context, query string, historyContext []history.HistoryEntry) (string, error) {
	prompt := buildFindPromptText(query, historyContext)
	if prompt == "" {
		return "(No relevant commands found or history was empty)", nil
	}
	result, err := c.chat(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("cloud API call failed (Find): %w", err)
	}
	if result == "" || result == "No relevant commands found." {
		return "(No relevant commands found or AI response was empty)", nil
	}
	return verifiedMatches(result, historyContext), nil
}

// SuggestCommands implements LLMClient.
func (c *CloudClient) SuggestCommands(ctx context.Context, taskDescription string, historyContext []history.HistoryEntry) (string, error) {
	prompt := buildSuggestPromptText(taskDescription, historyContext)
	result, err := c.chat(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("cloud API call failed (Suggest): %w", err)
	}
	if result == "" || result == "Cannot suggest a command for this task." {
		return "(AI could not suggest a command for this task)", nil
	}
	return result, nil
}

// chat sends a prompt to the cloud provider's OpenAI-compatible endpoint.
func (c *CloudClient) chat(ctx context.Context, prompt string) (string, error) {
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("cloud API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}
