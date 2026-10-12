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

type ollamaRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Think    *bool         `json:"think,omitempty"`
	Options  struct {
		NumPredict int `json:"num_predict"`
	} `json:"options"`
}

type ollamaResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
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
func (c *OllamaClient) SuggestCommands(ctx context.Context, taskDescription, shell string, historyContext []history.HistoryEntry) (string, error) {
	prompt := buildSuggestPromptText(taskDescription, shell, historyContext)
	result, err := c.chat(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("ollama API call failed (Suggest): %w", err)
	}
	if result == "" || result == "Cannot suggest a command for this task." {
		return "(AI could not suggest a command for this task)", nil
	}
	return result, nil
}

// chat sends a prompt to Ollama's native chat endpoint.
func (c *OllamaClient) chat(ctx context.Context, prompt string) (string, error) {
	reqBody := ollamaRequest{
		Model:    c.model,
		Messages: []chatMessage{{Role: "user", Content: prompt}},
		Stream:   false,
	}
	reqBody.Options.NumPredict = 1024
	if c.disableThinking {
		think := false
		reqBody.Think = &think
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.baseURL, "/")+"/api/chat", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxProviderResponseBytes))
		return "", fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, boundedErrorBody(bodyBytes))
	}

	var chatResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	result := strings.TrimSpace(chatResp.Message.Content)
	if c.disableThinking {
		result = stripThinkingTags(result)
	}
	return result, nil
}
