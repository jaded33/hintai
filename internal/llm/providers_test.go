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

func TestCloudProvidersUseCompatibleEndpoint(t *testing.T) {
	for provider := range cloudBaseURLs {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
					t.Errorf("path=%s", r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("missing auth")
				}
				var body chatRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Model != "test-model" || body.MaxTokens == 0 {
					t.Errorf("request=%+v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"choices":[{"message":{"content":"H1"}}]}`)
			}))
			defer server.Close()
			original := cloudBaseURLs[provider]
			cloudBaseURLs[provider] = server.URL
			defer func() { cloudBaseURLs[provider] = original }()
			c, err := NewCloudClient(context.Background(), zap.NewNop(), provider, "test-key", "test-model")
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.FindHistoryEntries(context.Background(), "find it", []history.HistoryEntry{{Command: "echo safe"}})
			if err != nil || got != "echo safe" {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func TestCloudRequestCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	c := &CloudClient{httpClient: server.Client(), baseURL: server.URL, apiKey: "k", model: "m"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := c.chat(ctx, "hello"); done <- err }()
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
	loaded := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/models" {
			loaded = true
			fmt.Fprint(w, `{"models":[{"type":"llm","key":"other","loaded_instances":[{"id":"other"}]},{"type":"llm","key":"wanted","loaded_instances":[{"id":"wanted"}]},{"type":"embedding","key":"embed","loaded_instances":[{"id":"embed"}]}]}`)
			return
		}
		t.Fatalf("unexpected request %s", r.URL.Path)
	}))
	defer server.Close()
	c := &LMStudioClient{httpClient: server.Client(), baseURL: server.URL, model: "wanted"}
	if got := c.getActiveModel(context.Background()); got != "wanted" || !loaded {
		t.Fatalf("got %q", got)
	}
}

func TestLMStudioLoadFailureIsReturned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models/load" {
			t.Errorf("path=%s", r.URL.Path)
		}
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer server.Close()
	c := &LMStudioClient{httpClient: server.Client(), baseURL: server.URL, model: "missing"}
	if err := c.loadModel(context.Background()); err == nil {
		t.Fatal("expected load failure")
	}
}

func TestOllamaChatRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()
	c := &OllamaClient{httpClient: server.Client(), baseURL: server.URL, model: "local"}
	got, err := c.chat(context.Background(), "hello")
	if err != nil || got != "ok" {
		t.Fatalf("%q, %v", got, err)
	}
}

func TestCloudProviderErrors(t *testing.T) {
	for provider := range cloudBaseURLs {
		for _, tc := range []struct {
			name   string
			status int
			body   string
		}{
			{"authentication", http.StatusUnauthorized, "{\"error\":\"bad key\"}"},
			{"malformed response", http.StatusOK, "{broken"},
		} {
			t.Run(provider+"/"+tc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
				}))
				defer server.Close()
				original := cloudBaseURLs[provider]
				cloudBaseURLs[provider] = server.URL
				defer func() { cloudBaseURLs[provider] = original }()
				c, err := NewCloudClient(context.Background(), zap.NewNop(), provider, "key", "model")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := c.chat(context.Background(), "hello"); err == nil {
					t.Fatal("expected provider error")
				}
			})
		}
	}
}
