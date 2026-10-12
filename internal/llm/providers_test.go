package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jaded33/hintai/internal/history"
	"go.uber.org/zap"
)

func TestCloudProviderRequestFormats(t *testing.T) {
	tests := []struct {
		provider string
		path     string
		response string
	}{
		{"openai", "/v1/responses", `{"output":[{"type":"message","content":[{"type":"output_text","text":"H1"}]}]}`},
		{"anthropic", "/v1/messages", `{"content":[{"type":"text","text":"H1"}]}`},
		{"gemini", "/v1/interactions", `{"status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"H1"}]}]}`},
		{"openrouter", "/v1/chat/completions", `{"choices":[{"message":{"content":"H1"}}]}`},
		{"grok", "/v1/chat/completions", `{"choices":[{"message":{"content":"H1"}}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path = %q, want %q", r.URL.Path, tc.path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("method = %q", r.Method)
				}
				checkProviderHeaders(t, tc.provider, r)
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				} else {
					checkProviderBody(t, tc.provider, body)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.response)
			}))
			defer server.Close()

			client, err := NewCloudClient(zap.NewNop(), tc.provider, server.URL+"/v1", "test-key", "test-model")
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.FindHistoryEntries(context.Background(), "find it", []history.HistoryEntry{{Command: "echo safe"}})
			if err != nil || got != "echo safe" {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func checkProviderHeaders(t *testing.T, provider string, r *http.Request) {
	t.Helper()
	switch provider {
	case "anthropic":
		if r.Header.Get("x-api-key") != "test-key" || r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("Anthropic headers missing")
		}
	case "gemini":
		if r.Header.Get("x-goog-api-key") != "test-key" || r.Header.Get("Api-Revision") != "2026-05-20" {
			t.Errorf("Gemini headers missing")
		}
	default:
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header missing")
		}
	}
}

func checkProviderBody(t *testing.T, provider string, body map[string]any) {
	t.Helper()
	if body["model"] != "test-model" {
		t.Errorf("model = %#v", body["model"])
	}
	switch provider {
	case "openai":
		if body["input"] == "" || body["store"] != false || body["max_output_tokens"] != float64(1024) {
			t.Errorf("unexpected Responses API body: %#v", body)
		}
	case "anthropic":
		if body["max_tokens"] != float64(1024) || !firstMessageHasPrompt(body["messages"]) {
			t.Errorf("unexpected Messages API body: %#v", body)
		}
	case "gemini":
		if body["input"] == "" || body["store"] != false {
			t.Errorf("unexpected Interactions API body: %#v", body)
		}
		generation, ok := body["generation_config"].(map[string]any)
		if !ok || generation["max_output_tokens"] != float64(1024) {
			t.Errorf("generation config = %#v", body["generation_config"])
		}
	case "openrouter", "grok":
		if body["max_tokens"] != float64(1024) || !firstMessageHasPrompt(body["messages"]) {
			t.Errorf("unexpected chat-completions body: %#v", body)
		}
	}
}

func firstMessageHasPrompt(messages any) bool {
	items, ok := messages.([]any)
	if !ok || len(items) == 0 {
		return false
	}
	message, ok := items[0].(map[string]any)
	return ok && message["role"] == "user" && message["content"] != ""
}

func TestCloudProviderErrors(t *testing.T) {
	providers := []string{"openai", "anthropic", "gemini", "openrouter", "grok"}
	for _, provider := range providers {
		for _, tc := range []struct {
			name   string
			status int
			body   string
		}{
			{"authentication", http.StatusUnauthorized, `{"error":"bad key"}`},
			{"unavailable model", http.StatusNotFound, `{"error":"unknown model"}`},
			{"malformed response", http.StatusOK, "{broken"},
		} {
			t.Run(provider+"/"+tc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
				}))
				defer server.Close()
				client, err := NewCloudClient(zap.NewNop(), provider, server.URL, "key", "model")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := client.chat(context.Background(), "hello"); err == nil {
					t.Fatal("expected provider error")
				}
			})
		}
	}
}

func TestCloudRequestCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	}))
	defer server.Close()
	defer close(release)
	client, err := NewCloudClient(zap.NewNop(), "openai", server.URL, "k", "m")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := client.chat(ctx, "hello"); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("request did not cancel")
	}
}

func TestLMStudioReusesConfiguredLoadedTextModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" {
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer lm-token" {
			t.Error("model list request did not use LM_API_TOKEN")
		}
		fmt.Fprint(w, `{"models":[{"type":"llm","key":"other","loaded_instances":[{"id":"other"}]},{"type":"llm","key":"wanted","loaded_instances":[{"id":"wanted"}]},{"type":"embedding","key":"embed","loaded_instances":[{"id":"embed"}]}]}`)
	}))
	defer server.Close()
	c := &LMStudioClient{httpClient: server.Client(), modelHTTPClient: server.Client(), baseURL: server.URL, model: "wanted", apiKey: "lm-token"}
	got, err := c.getActiveModel(context.Background())
	if err != nil || got != "wanted" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestLMStudioLoadFailureIsReturned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models/load" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer lm-token" {
			t.Error("load request did not use LM_API_TOKEN")
		}
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer server.Close()
	c := &LMStudioClient{httpClient: server.Client(), modelHTTPClient: server.Client(), baseURL: server.URL, model: "missing", apiKey: "lm-token"}
	if err := c.loadModel(context.Background()); err == nil {
		t.Fatal("expected load failure")
	}
}

func TestLMStudioLoadsConfiguredModelWhenNoneIsLoaded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			fmt.Fprint(w, `{"models":[]}`)
		case "/api/v1/models/load":
			var request loadModelRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.Model != "configured" {
				t.Errorf("loaded model = %q", request.Model)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewLMStudioClient(context.Background(), zap.NewNop(), server.URL, "configured", true, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if client.model != "configured" {
		t.Fatalf("model = %q", client.model)
	}
}

func TestLMStudioModelListUnauthorizedIsReturned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	c := &LMStudioClient{httpClient: server.Client(), modelHTTPClient: server.Client(), baseURL: server.URL}
	if _, err := c.getActiveModel(context.Background()); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected actionable authorization error, got %v", err)
	}
}

func TestOllamaNativeChatRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path=%s", r.URL.Path)
		}
		var body ollamaRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "local" || body.Stream || body.Think == nil || *body.Think || body.Options.NumPredict != 1024 {
			t.Errorf("request=%+v", body)
		}
		fmt.Fprint(w, `{"message":{"role":"assistant","content":"ok"},"done":true}`)
	}))
	defer server.Close()
	c := &OllamaClient{httpClient: server.Client(), baseURL: server.URL, model: "local", disableThinking: true}
	got, err := c.chat(context.Background(), "hello")
	if err != nil || got != "ok" {
		t.Fatalf("%q, %v", got, err)
	}
}

func TestLocalProviderMalformedResponses(t *testing.T) {
	for _, provider := range []string{"lmstudio", "ollama"} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, "{broken")
			}))
			defer server.Close()
			var chat func(context.Context) error
			if provider == "lmstudio" {
				client := &LMStudioClient{httpClient: server.Client(), baseURL: server.URL, model: "model"}
				chat = func(ctx context.Context) error { _, err := client.chat(ctx, "hello"); return err }
			} else {
				client := &OllamaClient{httpClient: server.Client(), baseURL: server.URL, model: "model"}
				chat = func(ctx context.Context) error { _, err := client.chat(ctx, "hello"); return err }
			}
			if err := chat(context.Background()); err == nil {
				t.Fatal("expected malformed response error")
			}
		})
	}
}

func TestLocalProviderRequestsAreCancelable(t *testing.T) {
	for _, provider := range []string{"lmstudio", "ollama"} {
		t.Run(provider, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-release
			}))
			defer server.Close()
			defer close(release)

			var chat func(context.Context) error
			if provider == "lmstudio" {
				client := &LMStudioClient{httpClient: server.Client(), baseURL: server.URL, model: "model"}
				chat = func(ctx context.Context) error { _, err := client.chat(ctx, "hello"); return err }
			} else {
				client := &OllamaClient{httpClient: server.Client(), baseURL: server.URL, model: "model"}
				chat = func(ctx context.Context) error { _, err := client.chat(ctx, "hello"); return err }
			}

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- chat(ctx) }()
			<-started
			cancel()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("expected cancellation error")
				}
			case <-time.After(time.Second):
				t.Fatal("request did not cancel")
			}
		})
	}
}

func TestProviderErrorResponseIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, strings.Repeat("x", maxProviderResponseBytes+100))
	}))
	defer server.Close()
	client, err := NewCloudClient(zap.NewNop(), "openai", server.URL, "k", "m")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.chat(context.Background(), "hello"); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("expected bounded response error, got %v", err)
	}
}
