//go:build integration

package ollama_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/cespare/xxhash/v2"

	ollamaapi "github.com/ollama/ollama/api"

	"github.com/dacbd/llm-proxy/internal/handler/ollama"
	"github.com/dacbd/llm-proxy/weave"
)

const (
	localOllama = "http://localhost:11434"
	testModel   = "qwen3.5:0.8b"
)

var noThink = &ollamaapi.ThinkValue{Value: false}

// hashString returns a xxhash64 hash of the input string as a hexadecimal string.
// This mirrors the function in handler.go for test consistency.
func hashString(s string) string {
	h := xxhash.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum64())
}

// newMock returns a MockAPI that records calls for assertion.
func newMock() *weave.MockAPI {
	return &weave.MockAPI{ProjectIDVal: "test-entity/test-project"}
}

func TestGenerate_Streaming(t *testing.T) {
	mock := newMock()
	var (
		mu          sync.Mutex
		startCalled bool
		endCalled   bool
		startInput  weave.StartedCallSchemaForInsert
		endInput    weave.EndedCallSchemaForInsert
	)
	mock.CallStartFn = func(_ context.Context, s weave.StartedCallSchemaForInsert) (weave.CallStartRes, error) {
		mu.Lock()
		startCalled = true
		startInput = s
		mu.Unlock()
		return weave.CallStartRes{ID: "test-id", TraceID: "test-trace"}, nil
	}
	mock.CallEndFn = func(_ context.Context, e weave.EndedCallSchemaForInsert) error {
		mu.Lock()
		endCalled = true
		endInput = e
		mu.Unlock()
		return nil
	}

	h := ollama.NewHandler(localOllama, mock)
	srv := httptest.NewServer(http.HandlerFunc(h.Generate))
	defer srv.Close()

	stream := true
	req := ollamaapi.GenerateRequest{
		Model:  testModel,
		Prompt: "Reply with one word: hello",
		Stream: &stream,
		Think:  noThink,
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

	var chunks []ollamaapi.GenerateResponse
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var chunk ollamaapi.GenerateResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			t.Fatalf("failed to parse response chunk: %v", err)
		}
		chunks = append(chunks, chunk)
		if chunk.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading response: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("received no response chunks")
	}

	last := chunks[len(chunks)-1]
	if !last.Done {
		t.Error("last chunk should have Done=true")
	}
	if last.Model != testModel {
		t.Errorf("expected model %q, got %q", testModel, last.Model)
	}
	if last.EvalCount == 0 {
		t.Error("expected non-zero EvalCount in final chunk")
	}

	var full string
	for _, c := range chunks {
		full += c.Response
	}
	if full == "" {
		t.Error("expected non-empty response text")
	}
	t.Logf("response (%d chunks, %d tokens): %s", len(chunks), last.EvalCount, full)

	// Allow background goroutines to complete.
	srv.Close()
	mu.Lock()
	defer mu.Unlock()

	if !startCalled {
		t.Error("expected weave CallStart to be called")
	}
	if startInput.OpName != "ollama.generate" {
		t.Errorf("expected op_name %q, got %q", "ollama.generate", startInput.OpName)
	}
	if startInput.Inputs["model"] != testModel {
		t.Errorf("expected input model %q, got %v", testModel, startInput.Inputs["model"])
	}
	if !endCalled {
		t.Error("expected weave CallEnd to be called")
	}
	if endInput.Summary.Usage == nil {
		t.Error("expected usage in CallEnd summary")
	}
}

func TestGenerate_NonStreaming(t *testing.T) {
	mock := newMock()
	h := ollama.NewHandler(localOllama, mock)
	srv := httptest.NewServer(http.HandlerFunc(h.Generate))
	defer srv.Close()

	stream := false
	req := ollamaapi.GenerateRequest{
		Model:  testModel,
		Prompt: "Reply with one word: hello",
		Stream: &stream,
		Think:  noThink,
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

	var genResp ollamaapi.GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !genResp.Done {
		t.Error("expected Done=true in non-streaming response")
	}
	if genResp.Response == "" {
		t.Error("expected non-empty response text")
	}
	if genResp.Model != testModel {
		t.Errorf("expected model %q, got %q", testModel, genResp.Model)
	}
	t.Logf("response (%d tokens): %s", genResp.EvalCount, genResp.Response)
}

func TestGenerate_InvalidBody(t *testing.T) {
	h := ollama.NewHandler(localOllama, nil)
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

func TestGenerate_InvalidUpstreamURL(t *testing.T) {
	h := ollama.NewHandler("://bad-url", nil)
	srv := httptest.NewServer(http.HandlerFunc(h.Generate))
	defer srv.Close()

	body, _ := json.Marshal(ollamaapi.GenerateRequest{Model: testModel, Prompt: "hi"})
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
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

func TestGenerate_WeaveDisabled(t *testing.T) {
	// nil weave client should not panic
	h := ollama.NewHandler(localOllama, nil)
	srv := httptest.NewServer(http.HandlerFunc(h.Generate))
	defer srv.Close()

	stream := false
	body, _ := json.Marshal(ollamaapi.GenerateRequest{
		Model:  testModel,
		Prompt: "hi",
		Stream: &stream,
		Think:  noThink,
	})
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestChat_Streaming(t *testing.T) {
	mock := newMock()
	h := ollama.NewHandler(localOllama, mock)
	srv := httptest.NewServer(http.HandlerFunc(h.Chat))
	defer srv.Close()

	stream := true
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

	var chunks []ollamaapi.ChatResponse
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var chunk ollamaapi.ChatResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			t.Fatalf("failed to parse response chunk: %v", err)
		}
		chunks = append(chunks, chunk)
		if chunk.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading response: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("received no response chunks")
	}

	last := chunks[len(chunks)-1]
	if !last.Done {
		t.Error("last chunk should have Done=true")
	}
	if last.Model != testModel {
		t.Errorf("expected model %q, got %q", testModel, last.Model)
	}
	if last.Metrics.EvalCount == 0 {
		t.Error("expected non-zero EvalCount in final chunk")
	}

	var full string
	for _, c := range chunks {
		full += c.Message.Content
	}
	if full == "" {
		t.Error("expected non-empty response text")
	}
	t.Logf("response (%d chunks, %d tokens): %s", len(chunks), last.Metrics.EvalCount, full)
}

func TestChat_NonStreaming(t *testing.T) {
	mock := newMock()
	h := ollama.NewHandler(localOllama, mock)
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
	t.Logf("response (%d tokens): %s", chatResp.Metrics.EvalCount, chatResp.Message.Content)
}

func TestChat_InvalidBody(t *testing.T) {
	h := ollama.NewHandler(localOllama, nil)
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

func TestChat_WeaveDisabled(t *testing.T) {
	// nil weave client should not panic
	h := ollama.NewHandler(localOllama, nil)
	srv := httptest.NewServer(http.HandlerFunc(h.Chat))
	defer srv.Close()

	stream := false
	messages := []ollamaapi.Message{{Role: "user", Content: "hi"}}
	body, _ := json.Marshal(ollamaapi.ChatRequest{
		Model:    testModel,
		Messages: messages,
		Stream:   &stream,
	})
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// TestChat_WithTracing verifies that tracing works correctly for chat requests
func TestChat_WithTracing(t *testing.T) {
	// Create a mock upstream server that returns a simple chat response
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a streaming chat response
		w.Header().Set("Content-Type", "application/x-ndjson")
		resp := ollamaapi.ChatResponse{
			Model: testModel,
			Message: ollamaapi.Message{
				Role:    "assistant",
				Content: "Hello! How can I help you today?",
			},
			Done: true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	mock := newMock()
	var (
		mu          sync.Mutex
		startCalled bool
		endCalled   bool
		startInput  weave.StartedCallSchemaForInsert
		_           weave.EndedCallSchemaForInsert
	)
	mock.CallStartFn = func(_ context.Context, s weave.StartedCallSchemaForInsert) (weave.CallStartRes, error) {
		mu.Lock()
		startCalled = true
		startInput = s
		mu.Unlock()
		return weave.CallStartRes{ID: "test-id", TraceID: "test-trace"}, nil
	}
	mock.CallEndFn = func(_ context.Context, e weave.EndedCallSchemaForInsert) error {
		mu.Lock()
		endCalled = true
		mu.Unlock()
		return nil
	}
	// Mock CallRead to return nil (parent span doesn't exist initially)
	mock.CallReadFn = func(_ context.Context, callID string) (*weave.CallSchema, error) {
		return nil, fmt.Errorf("not found")
	}

	h := ollama.NewHandler(upstream.URL, mock)
	srv := httptest.NewServer(http.HandlerFunc(h.Chat))
	defer srv.Close()

	// Test with multiple messages to trigger parent span creation
	messages := []ollamaapi.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "How are you?"},
	}
	stream := false
	req := ollamaapi.ChatRequest{
		Model:    testModel,
		Messages: messages,
		Stream:   &stream,
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

	// Allow background goroutines to complete.
	srv.Close()
	mu.Lock()
	defer mu.Unlock()

	if !startCalled {
		t.Error("expected weave CallStart to be called")
	}
	if startInput.OpName != "ollama.chat" {
		t.Errorf("expected op_name %q, got %q", "ollama.chat", startInput.OpName)
	}
	// Check that the ID is derived from the latest message
	expectedID := hashString(messages[len(messages)-1].Content)
	if startInput.ID == nil || *startInput.ID != expectedID {
		t.Errorf("expected ID %q (hash of latest message), got %v", expectedID, startInput.ID)
	}
	// Check that parent ID is derived from the first message
	if len(messages) > 1 {
		expectedParentID := hashString(messages[0].Content)
		if startInput.ParentID == nil || *startInput.ParentID != expectedParentID {
			t.Errorf("expected parent ID %q (hash of first message), got %v", expectedParentID, startInput.ParentID)
		}
	}
	if !endCalled {
		t.Error("expected weave CallEnd to be called")
	}
}

// TestChat_SingleMessage verifies tracing works with a single message (no parent)
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
			// Create a mock upstream server to capture received headers
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
			// Create a mock upstream server to capture received headers
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
			// Create a mock upstream server to capture received headers
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

func TestChat_SingleMessage(t *testing.T) {
	// Create a mock upstream server that returns a simple chat response
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a streaming chat response
		w.Header().Set("Content-Type", "application/x-ndjson")
		resp := ollamaapi.ChatResponse{
			Model: testModel,
			Message: ollamaapi.Message{
				Role:    "assistant",
				Content: "Hello! How can I help you today?",
			},
			Done: true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	mock := newMock()
	var (
		mu          sync.Mutex
		startCalled bool
		endCalled   bool
		startInput  weave.StartedCallSchemaForInsert
		_           weave.EndedCallSchemaForInsert
	)
	mock.CallStartFn = func(_ context.Context, s weave.StartedCallSchemaForInsert) (weave.CallStartRes, error) {
		mu.Lock()
		startCalled = true
		startInput = s
		mu.Unlock()
		return weave.CallStartRes{ID: "test-id", TraceID: "test-trace"}, nil
	}
	mock.CallEndFn = func(_ context.Context, e weave.EndedCallSchemaForInsert) error {
		mu.Lock()
		endCalled = true
		mu.Unlock()
		return nil
	}

	h := ollama.NewHandler(upstream.URL, mock)
	srv := httptest.NewServer(http.HandlerFunc(h.Chat))
	defer srv.Close()

	// Test with single message (no parent)
	messages := []ollamaapi.Message{
		{Role: "user", Content: "Single message"},
	}
	stream := false
	req := ollamaapi.ChatRequest{
		Model:    testModel,
		Messages: messages,
		Stream:   &stream,
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

	// Allow background goroutines to complete.
	srv.Close()
	mu.Lock()
	defer mu.Unlock()

	if !startCalled {
		t.Error("expected weave CallStart to be called")
	}
	// Check that the ID is derived from the message
	expectedID := hashString(messages[0].Content)
	if startInput.ID == nil || *startInput.ID != expectedID {
		t.Errorf("expected ID %q (hash of message), got %v", expectedID, startInput.ID)
	}
	// For single message, there should be no parent ID
	if startInput.ParentID != nil {
		t.Errorf("expected no parent ID for single message, got %v", startInput.ParentID)
	}
	if !endCalled {
		t.Error("expected weave CallEnd to be called")
	}
}
