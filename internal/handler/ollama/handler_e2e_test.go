//go:build integration && e2e

package ollama_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	ollamaapi "github.com/ollama/ollama/api"

	"github.com/dacbd/llm-proxy/internal/handler/ollama"
	"github.com/dacbd/llm-proxy/weave"
)

const (
	localOllama  = "http://localhost:11434"
	e2eTestModel = "qwen3.5:0.8b"
)

var noThink = &ollamaapi.ThinkValue{Value: false}

// newE2EMock returns a MockAPI that records calls for assertion.
func newE2EMock() *weave.MockAPI {
	return &weave.MockAPI{ProjectIDVal: "test-entity/test-project"}
}

func TestGenerate_Streaming(t *testing.T) {
	mock := newE2EMock()
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
		Model:  e2eTestModel,
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
	if last.Model != e2eTestModel {
		t.Errorf("expected model %q, got %q", e2eTestModel, last.Model)
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
	if startInput.Inputs["model"] != e2eTestModel {
		t.Errorf("expected input model %q, got %v", e2eTestModel, startInput.Inputs["model"])
	}
	if !endCalled {
		t.Error("expected weave CallEnd to be called")
	}
	if endInput.Summary.Usage == nil {
		t.Error("expected usage in CallEnd summary")
	}
}

func TestGenerate_NonStreaming(t *testing.T) {
	mock := newE2EMock()
	h := ollama.NewHandler(localOllama, mock)
	srv := httptest.NewServer(http.HandlerFunc(h.Generate))
	defer srv.Close()

	stream := false
	req := ollamaapi.GenerateRequest{
		Model:  e2eTestModel,
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
	if genResp.Model != e2eTestModel {
		t.Errorf("expected model %q, got %q", e2eTestModel, genResp.Model)
	}
	t.Logf("response (%d tokens): %s", genResp.EvalCount, genResp.Response)
}

func TestGenerate_WeaveDisabled(t *testing.T) {
	// nil weave client should not panic
	h := ollama.NewHandler(localOllama, nil)
	srv := httptest.NewServer(http.HandlerFunc(h.Generate))
	defer srv.Close()

	stream := false
	body, _ := json.Marshal(ollamaapi.GenerateRequest{
		Model:  e2eTestModel,
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
	mock := newE2EMock()
	h := ollama.NewHandler(localOllama, mock)
	srv := httptest.NewServer(http.HandlerFunc(h.Chat))
	defer srv.Close()

	stream := true
	req := ollamaapi.ChatRequest{
		Model: e2eTestModel,
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
	if last.Model != e2eTestModel {
		t.Errorf("expected model %q, got %q", e2eTestModel, last.Model)
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

func TestChat_WeaveDisabled(t *testing.T) {
	// nil weave client should not panic
	h := ollama.NewHandler(localOllama, nil)
	srv := httptest.NewServer(http.HandlerFunc(h.Chat))
	defer srv.Close()

	stream := false
	messages := []ollamaapi.Message{{Role: "user", Content: "hi"}}
	body, _ := json.Marshal(ollamaapi.ChatRequest{
		Model:    e2eTestModel,
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
