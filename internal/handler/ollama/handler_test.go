package ollama_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ollamaapi "github.com/ollama/ollama/api"

	"github.com/dacbd/llm-proxy/internal/handler/ollama"
	"github.com/dacbd/llm-proxy/weave"
)

const testModel = "qwen3.5:0.8b"

// newMock returns a MockAPI that records calls for assertion.
func newMock() *weave.MockAPI {
	return &weave.MockAPI{ProjectIDVal: "test-entity/test-project"}
}

func TestChat_NonStreaming_Mock(t *testing.T) {
	// Create a mock upstream server that returns a simple chat response
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := ollamaapi.ChatResponse{
			Model: testModel,
			Message: ollamaapi.Message{
				Role:    "assistant",
				Content: "Hello! How can I help you today?",
			},
			Done: true,
			Metrics: ollamaapi.Metrics{
				EvalCount: 10,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	mock := newMock()
	h := ollama.NewHandler(upstream.URL, mock)
	srv := httptest.NewServer(http.HandlerFunc(h.Chat))
	defer srv.Close()

	stream := false
	req := ollamaapi.ChatRequest{
		Model: testModel,
		Messages: []ollamaapi.Message{
			{Role: "user", Content: "Reply with one word: hello"},
		},
		Stream: &stream,
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var chatResp ollamaapi.ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !chatResp.Done {
		t.Error("expected Done=true in non-streaming response")
	}
	if chatResp.Message.Content == "" {
		t.Error("expected non-empty response text")
	}
	if chatResp.Model != testModel {
		t.Errorf("expected model %q, got %q", testModel, chatResp.Model)
	}
}

func TestGenerate_InvalidBody(t *testing.T) {
	// Create a mock upstream (not used for this test)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	h := ollama.NewHandler(upstream.URL, nil)
	srv := httptest.NewServer(http.HandlerFunc(h.Generate))
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader([]byte("not json")))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGenerate_UpstreamUnavailable(t *testing.T) {
	ghost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ghost.Close()

	h := ollama.NewHandler(ghost.URL, nil)
	srv := httptest.NewServer(http.HandlerFunc(h.Generate))
	defer srv.Close()

	body, _ := json.Marshal(ollamaapi.GenerateRequest{Model: testModel, Prompt: "hi"})
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", resp.StatusCode)
	}
}

func TestGenerate_MalformedUpstreamResponse(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-json\n"))
		json.NewEncoder(w).Encode(ollamaapi.GenerateResponse{Model: testModel, Done: true})
	}))
	defer fake.Close()

	h := ollama.NewHandler(fake.URL, nil)
	srv := httptest.NewServer(http.HandlerFunc(h.Generate))
	defer srv.Close()

	body, _ := json.Marshal(ollamaapi.GenerateRequest{Model: testModel, Prompt: "hi"})
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestChat_InvalidBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	h := ollama.NewHandler(upstream.URL, nil)
	srv := httptest.NewServer(http.HandlerFunc(h.Chat))
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader([]byte("not json")))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestChat_UpstreamUnavailable(t *testing.T) {
	ghost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ghost.Close()

	h := ollama.NewHandler(ghost.URL, nil)
	srv := httptest.NewServer(http.HandlerFunc(h.Chat))
	defer srv.Close()

	messages := []ollamaapi.Message{{Role: "user", Content: "hi"}}
	body, _ := json.Marshal(ollamaapi.ChatRequest{Model: testModel, Messages: messages})
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", resp.StatusCode)
	}
}

func TestChat_InvalidUpstreamURL(t *testing.T) {
	h := ollama.NewHandler("://bad-url", nil)
	srv := httptest.NewServer(http.HandlerFunc(h.Chat))
	defer srv.Close()

	messages := []ollamaapi.Message{{Role: "user", Content: "hi"}}
	body, _ := json.Marshal(ollamaapi.ChatRequest{Model: testModel, Messages: messages})
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestCopyRequestHeaders_Forwarded(t *testing.T) {
	testCases := []struct {
		name   string
		header string
		value  string
	}{
		{"Authorization", "Authorization", "Bearer test-token-123"},
		{"X-Request-Id", "X-Request-Id", "req-abc-123"},
		{"X-Correlation-Id", "X-Correlation-Id", "corr-xyz-789"},
		{"Accept", "Accept", "application/json"},
		{"User-Agent", "User-Agent", "Custom-Agent/1.0"},
		{"X-Requested-With", "X-Requested-With", "XMLHttpRequest"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var receivedHeader string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeader = r.Header.Get(tc.header)
				w.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()

			h := ollama.NewHandler(upstream.URL, nil)
			srv := httptest.NewServer(http.HandlerFunc(h.Generate))
			defer srv.Close()

			req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader([]byte(`{"model":"test","prompt":"hi"}`)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(tc.header, tc.value)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if receivedHeader != tc.value {
				t.Errorf("expected %s header %q, got %q", tc.header, tc.value, receivedHeader)
			}
		})
	}
}

func TestCopyRequestHeaders_OpenAICompatibility(t *testing.T) {
	testCases := []struct {
		name   string
		header string
		value  string
	}{
		{"OpenAI-Beta", "OpenAI-Beta", "assistants=v2"},
		{"X-Stainless-Lang", "x-stainless-lang", "go"},
		{"X-Stainless-Arch", "X-Stainless-Arch", "arm64"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var receivedHeader string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeader = r.Header.Get(tc.header)
				w.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()

			h := ollama.NewHandler(upstream.URL, nil)
			srv := httptest.NewServer(http.HandlerFunc(h.Generate))
			defer srv.Close()

			req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader([]byte(`{"model":"test","prompt":"hi"}`)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(tc.header, tc.value)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if receivedHeader != tc.value {
				t.Errorf("expected %s header %q, got %q", tc.header, tc.value, receivedHeader)
			}
		})
	}
}

func TestCopyRequestHeaders_Ignored(t *testing.T) {
	testCases := []struct {
		name   string
		header string
		value  string
	}{
		{"Cookie", "Cookie", "session=abc123"},
		{"Referer", "Referer", "http://example.com"},
		{"X-Custom-Header", "X-Custom-Header", "custom-value"},
		{"Cache-Control", "Cache-Control", "no-cache"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var receivedHeader string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeader = r.Header.Get(tc.header)
				w.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()

			h := ollama.NewHandler(upstream.URL, nil)
			srv := httptest.NewServer(http.HandlerFunc(h.Generate))
			defer srv.Close()

			req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader([]byte(`{"model":"test","prompt":"hi"}`)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(tc.header, tc.value)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if receivedHeader != "" {
				t.Errorf("expected %s header to be ignored (empty), got %q", tc.header, receivedHeader)
			}
		})
	}
}
