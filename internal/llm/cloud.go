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

	"github.com/jaded33/hintai/internal/config"
	"github.com/jaded33/hintai/internal/history"
	"go.uber.org/zap"
)

const maxProviderResponseBytes = 1 << 20

func boundedErrorBody(body []byte) string {
	message := strings.TrimSpace(string(body))
	if len(message) > 4096 {
		return message[:4096] + "…"
	}
	return message
}

// CloudClient handles the small set of provider-specific request and response
// formats used by HintAI's supported cloud APIs.
type CloudClient struct {
	logger     *zap.Logger
	httpClient *http.Client
	provider   string
	baseURL    string
	apiKey     string
	model      string
}

func NewCloudClient(logger *zap.Logger, provider, baseURL, apiKey, model string) (*CloudClient, error) {
	if config.DefaultBaseURL(provider) == "" || provider == "lmstudio" || provider == "ollama" {
		return nil, fmt.Errorf("unknown cloud provider %q", provider)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("set %s or api_key in config", config.APIKeyEnvironment(provider))
	}
	if baseURL == "" {
		baseURL = config.DefaultBaseURL(provider)
	}
	logger.Info("Using cloud provider", zap.String("provider", provider), zap.String("model", model))
	return &CloudClient{
		logger:     logger,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		provider:   provider,
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:     apiKey,
		model:      model,
	}, nil
}

func (c *CloudClient) Close() error { return nil }

func (c *CloudClient) FindHistoryEntries(ctx context.Context, query string, historyContext []history.HistoryEntry) (string, error) {
	prompt := buildFindPromptText(query, historyContext)
	if prompt == "" {
		return "(No relevant commands found or history was empty)", nil
	}
	result, err := c.chat(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("%s API call failed (Find): %w", c.provider, err)
	}
	if result == "" || result == "No relevant commands found." {
		return "(No relevant commands found or AI response was empty)", nil
	}
	return verifiedMatches(result, historyContext), nil
}

func (c *CloudClient) SuggestCommands(ctx context.Context, taskDescription, shell string, historyContext []history.HistoryEntry) (string, error) {
	prompt := buildSuggestPromptText(taskDescription, shell, historyContext)
	result, err := c.chat(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("%s API call failed (Suggest): %w", c.provider, err)
	}
	if result == "" || result == "Cannot suggest a command for this task." {
		return "(AI could not suggest a command for this task)", nil
	}
	return result, nil
}

func (c *CloudClient) chat(ctx context.Context, prompt string) (string, error) {
	switch c.provider {
	case "openai":
		return c.chatOpenAI(ctx, prompt)
	case "anthropic":
		return c.chatAnthropic(ctx, prompt)
	case "gemini":
		return c.chatGemini(ctx, prompt)
	case "openrouter", "grok":
		return c.chatOpenAICompatible(ctx, prompt)
	default:
		return "", fmt.Errorf("unsupported cloud provider %q", c.provider)
	}
}

type openAIRequest struct {
	Model           string `json:"model"`
	Input           string `json:"input"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	Store           bool   `json:"store"`
}

type openAIResponse struct {
	OutputText string `json:"output_text"`
	Output     []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

func (c *CloudClient) chatOpenAI(ctx context.Context, prompt string) (string, error) {
	body, err := c.doJSON(ctx, "/responses", openAIRequest{
		Model: c.model, Input: prompt, MaxOutputTokens: 1024, Store: false,
	}, map[string]string{"Authorization": "Bearer " + c.apiKey})
	if err != nil {
		return "", err
	}
	var response openAIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode OpenAI response: %w", err)
	}
	if response.OutputText != "" {
		return strings.TrimSpace(response.OutputText), nil
	}
	var text strings.Builder
	for _, item := range response.Output {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" {
				text.WriteString(content.Text)
			}
		}
	}
	return strings.TrimSpace(text.String()), nil
}

type anthropicRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Messages  []chatMessage `json:"messages"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func (c *CloudClient) chatAnthropic(ctx context.Context, prompt string) (string, error) {
	body, err := c.doJSON(ctx, "/messages", anthropicRequest{
		Model: c.model, MaxTokens: 1024, Messages: []chatMessage{{Role: "user", Content: prompt}},
	}, map[string]string{
		"x-api-key":         c.apiKey,
		"anthropic-version": "2023-06-01",
	})
	if err != nil {
		return "", err
	}
	var response anthropicResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode Anthropic response: %w", err)
	}
	var text strings.Builder
	for _, content := range response.Content {
		if content.Type == "text" {
			text.WriteString(content.Text)
		}
	}
	return strings.TrimSpace(text.String()), nil
}

type geminiRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Store          bool   `json:"store"`
	GenerationConf struct {
		MaxOutputTokens int `json:"max_output_tokens"`
	} `json:"generation_config"`
}

type geminiResponse struct {
	Status string `json:"status"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Steps []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"steps"`
}

func (c *CloudClient) chatGemini(ctx context.Context, prompt string) (string, error) {
	request := geminiRequest{Model: c.model, Input: prompt, Store: false}
	request.GenerationConf.MaxOutputTokens = 1024
	body, err := c.doJSON(ctx, "/interactions", request, map[string]string{
		"x-goog-api-key": c.apiKey,
		"Api-Revision":   "2026-05-20",
	})
	if err != nil {
		return "", err
	}
	var response geminiResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode Gemini response: %w", err)
	}
	if response.Status == "failed" {
		if len(response.Errors) > 0 && response.Errors[0].Message != "" {
			return "", fmt.Errorf("Gemini interaction failed: %s", response.Errors[0].Message)
		}
		return "", fmt.Errorf("Gemini interaction failed")
	}
	var text strings.Builder
	for _, step := range response.Steps {
		if step.Type != "model_output" {
			continue
		}
		for _, content := range step.Content {
			if content.Type == "text" {
				text.WriteString(content.Text)
			}
		}
	}
	return strings.TrimSpace(text.String()), nil
}

func (c *CloudClient) chatOpenAICompatible(ctx context.Context, prompt string) (string, error) {
	body, err := c.doJSON(ctx, "/chat/completions", chatRequest{
		Model: c.model, MaxTokens: 1024, Messages: []chatMessage{{Role: "user", Content: prompt}},
	}, map[string]string{"Authorization": "Bearer " + c.apiKey})
	if err != nil {
		return "", err
	}
	var response chatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode %s response: %w", c.provider, err)
	}
	if len(response.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}

func (c *CloudClient) doJSON(ctx context.Context, path string, requestBody any, headers map[string]string) ([]byte, error) {
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	response, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read provider response: %w", err)
	}
	if len(response) > maxProviderResponseBytes {
		return nil, fmt.Errorf("provider response exceeded %d bytes", maxProviderResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s returned status %d: %s", c.provider, resp.StatusCode, boundedErrorBody(response))
	}
	return response, nil
}
