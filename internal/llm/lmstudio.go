package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jaded33/hintai/internal/history"
	"go.uber.org/zap"
)

const (
	modelLoadTimeout = 30 * time.Second
	requestTimeout   = 60 * time.Second
)

// LMStudioClient implements the LLMClient interface using a local LM Studio server.
type LMStudioClient struct {
	logger          *zap.Logger
	httpClient      *http.Client
	baseURL         string
	model           string
	disableThinking bool
}

// chatRequest is the JSON body for /v1/chat/completions
type chatRequest struct {
	MaxTokens int           `json:"max_tokens,omitempty"`
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
}

// chatMessage is a single message in the chat
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the response from /v1/chat/completions
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// loadModelRequest is the JSON body for loading a model
type loadModelRequest struct {
	Model string `json:"model"`
}

// NewLMStudioClient creates a new LM Studio client and ensures the model is loaded.
func NewLMStudioClient(ctx context.Context, logger *zap.Logger, baseURL, defaultModel string, reuseLoaded, disableThinking bool) (*LMStudioClient, error) {
	c := &LMStudioClient{
		logger:          logger,
		httpClient:      &http.Client{Timeout: requestTimeout},
		baseURL:         baseURL,
		model:           defaultModel,
		disableThinking: disableThinking,
	}

	if reuseLoaded {
		if active := c.getActiveModel(ctx); active != "" {
			c.logger.Info("Using already-loaded model", zap.String("model", active))
			c.model = active
			return c, nil
		}
	}

	if c.model == "" {
		return nil, fmt.Errorf("no LM Studio model is loaded; set default_model or load one in LM Studio")
	}
	c.logger.Info("No model loaded, loading configured default", zap.String("model", c.model))
	if err := c.loadModel(ctx); err != nil {
		return nil, fmt.Errorf("failed to load LM Studio model: %w", err)
	}
	return c, nil
}

// getActiveModel sends a minimal test request to LM Studio.
// Returns the model ID that responded, or "" if nothing is loaded.
func (c *LMStudioClient) getActiveModel(ctx context.Context) string {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/models", nil)
	if err != nil {
		return ""
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var data struct {
		Models []struct {
			Type   string `json:"type"`
			Key    string `json:"key"`
			Loaded []struct {
				ID string `json:"id"`
			} `json:"loaded_instances"`
		} `json:"models"`
	}
	if json.NewDecoder(resp.Body).Decode(&data) != nil {
		return ""
	}
	fallback := ""
	for _, m := range data.Models {
		if m.Type != "llm" || len(m.Loaded) == 0 {
			continue
		}
		id := m.Loaded[0].ID
		if id == "" {
			id = m.Key
		}
		if id == c.model || m.Key == c.model {
			return id
		}
		if fallback == "" {
			fallback = id
		}
	}
	return fallback
}

// loadModel calls LM Studio to load the configured default model into memory.
func (c *LMStudioClient) loadModel(ctx context.Context) error {
	body, err := json.Marshal(loadModelRequest{Model: c.model})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/models/load", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("LM Studio model load request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("LM Studio model load returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	return nil
}

// Close is a no-op for LM Studio (no persistent connection to close).
func (c *LMStudioClient) Close() error {
	return nil
}

// FindHistoryEntries implements LLMClient.
func (c *LMStudioClient) FindHistoryEntries(ctx context.Context, query string, historyContext []history.HistoryEntry) (string, error) {
	prompt := buildFindPromptText(query, historyContext)
	if prompt == "" {
		return "(No relevant commands found or history was empty)", nil
	}
	result, err := c.chat(ctx, prompt)
	if err != nil {
		c.logger.Error("LM Studio call failed for FindHistoryEntries", zap.Error(err))
		return "", fmt.Errorf("lmstudio API call failed (Find): %w", err)
	}
	if result == "" || result == "No relevant commands found." {
		return "(No relevant commands found or AI response was empty)", nil
	}
	return verifiedMatches(result, historyContext), nil
}

// SuggestCommands implements LLMClient.
func (c *LMStudioClient) SuggestCommands(ctx context.Context, taskDescription string, historyContext []history.HistoryEntry) (string, error) {
	prompt := buildSuggestPromptText(taskDescription, historyContext)
	result, err := c.chat(ctx, prompt)
	if err != nil {
		c.logger.Error("LM Studio call failed for SuggestCommands", zap.Error(err))
		return "", fmt.Errorf("lmstudio API call failed (Suggest): %w", err)
	}
	if result == "" || result == "Cannot suggest a command for this task." {
		return "(AI could not suggest a command for this task or the response was empty)", nil
	}
	return result, nil
}

// chat sends a prompt to LM Studio and returns the text response.
func (c *LMStudioClient) chat(ctx context.Context, prompt string) (string, error) {
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
		return "", fmt.Errorf("LM Studio returned status %d: %s", resp.StatusCode, string(bodyBytes))
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

// stripThinkingTags removes <think>...</think> and similar reasoning blocks from LLM output.
func stripThinkingTags(s string) string {
	re := regexp.MustCompile(`(?si)<(think|thinking|reason|reasoning)>.*?</(think|thinking|reason|reasoning)>`)
	return strings.TrimSpace(re.ReplaceAllString(s, ""))
}
