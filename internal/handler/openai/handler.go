package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/dacbd/llm-proxy/weave"
)

type Handler struct {
	upstreamURL string
	client      *http.Client
	weave       weave.API
}

func NewHandler(upstreamURL string, weaveClient weave.API) *Handler {
	return &Handler{
		upstreamURL: upstreamURL,
		client:      &http.Client{},
		weave:       weaveClient,
	}
}

// ChatCompletions handles POST /v1/chat/completions
func (h *Handler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("failed to read request body", "error", err)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.Error("failed to parse request body", "error", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	slog.Info("chat completions request", "model", req.Model, "messages_count", len(req.Messages))

	// Handle Weave tracing if enabled
	var (
		startedAt = time.Now()
		callID    string
		parentID  *string
		startDone <-chan struct{}
	)

	if h.weave != nil && len(req.Messages) > 0 {
		latestMsg := req.Messages[len(req.Messages)-1]
		callID = hashString(latestMsg.Content)

		if len(req.Messages) > 1 {
			firstMsg := req.Messages[0]
			parentIDStr := hashString(firstMsg.Content)
			parentID = &parentIDStr
			go h.ensureParentSpanExists(context.Background(), *parentID, firstMsg)
		}

		startDone = h.traceStartChat(context.Background(), callID, parentID, startedAt, req)
	}

	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.upstreamURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		slog.Error("failed to create upstream request", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	upstream.Header.Set("Content-Type", "application/json")
	CopyRequestHeaders(upstream, r)

	resp, err := h.client.Do(upstream)
	if err != nil {
		slog.Error("upstream request failed", "error", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w, resp)

	// Handle streaming vs non-streaming responses
	isStreaming := req.Stream

	if isStreaming {
		h.streamResponse(w, resp.Body, callID, startDone, true)
	} else {
		h.proxyResponse(w, resp.Body, callID, startDone)
	}
}

// Completions handles POST /v1/completions (legacy)
func (h *Handler) Completions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("failed to read request body", "error", err)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req CompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.Error("failed to parse request body", "error", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	slog.Info("completions request", "model", req.Model, "prompt_len", len(req.Prompt))

	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.upstreamURL+"/v1/completions", bytes.NewReader(body))
	if err != nil {
		slog.Error("failed to create upstream request", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	upstream.Header.Set("Content-Type", "application/json")
	CopyRequestHeaders(upstream, r)

	resp, err := h.client.Do(upstream)
	if err != nil {
		slog.Error("upstream request failed", "error", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w, resp)

	// Handle streaming vs non-streaming responses
	isStreaming := req.Stream

	if isStreaming {
		h.streamResponse(w, resp.Body, "", nil, true)
	} else {
		h.proxyResponse(w, resp.Body, "", nil)
	}
}

// Embeddings handles POST /v1/embeddings
func (h *Handler) Embeddings(w http.ResponseWriter, r *http.Request) {
	slog.Info("embeddings request")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("failed to read request body", "error", err)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.upstreamURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		slog.Error("failed to create upstream request", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	upstream.Header.Set("Content-Type", "application/json")
	CopyRequestHeaders(upstream, r)

	resp, err := h.client.Do(upstream)
	if err != nil {
		slog.Error("upstream request failed", "error", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w, resp)
	h.proxyResponse(w, resp.Body, "", nil)
}

// ListModels handles GET /v1/models
func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	slog.Info("list models request")

	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodGet, h.upstreamURL+"/v1/models", nil)
	if err != nil {
		slog.Error("failed to create upstream request", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	CopyRequestHeaders(upstream, r)

	resp, err := h.client.Do(upstream)
	if err != nil {
		slog.Error("upstream request failed", "error", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w, resp)
	h.proxyResponse(w, resp.Body, "", nil)
}

// CreateImage handles POST /v1/images/generations
func (h *Handler) CreateImage(w http.ResponseWriter, r *http.Request) {
	slog.Info("image generation request")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("failed to read request body", "error", err)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.upstreamURL+"/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		slog.Error("failed to create upstream request", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	upstream.Header.Set("Content-Type", "application/json")
	CopyRequestHeaders(upstream, r)

	resp, err := h.client.Do(upstream)
	if err != nil {
		slog.Error("upstream request failed", "error", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w, resp)
	h.proxyResponse(w, resp.Body, "", nil)
}

// streamResponse handles SSE streaming responses
func (h *Handler) streamResponse(w http.ResponseWriter, respBody io.Reader, callID string, startDone <-chan struct{}, closeWhenDone bool) {
	flusher, canFlush := w.(http.Flusher)
	scanner := bufio.NewScanner(respBody)
	var fullResponse string

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}

		// OpenAI format: "data: {...}\n\n"
		if !bytes.HasPrefix([]byte(line), []byte("data: ")) {
			// Pass through non-data lines
			io.WriteString(w, line+"\n")
			if canFlush {
				flusher.Flush()
			}
			continue
		}

		// Extract JSON from "data: {...}"
		jsonData := bytes.TrimPrefix([]byte(line), []byte("data: "))
		if string(jsonData) == "[DONE]" {
			io.WriteString(w, line+"\n\n")
			if canFlush {
				flusher.Flush()
			}
			break
		}

		// Accumulate response for tracing
		if h.weave != nil && callID != "" {
			var chunk ChatCompletionChunk
			if err := json.Unmarshal(jsonData, &chunk); err == nil {
				if len(chunk.Choices) > 0 {
					fullResponse += chunk.Choices[0].Delta.Content
				}
			}
		}

		io.WriteString(w, line+"\n\n")
		if canFlush {
			flusher.Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Error("error reading upstream response", "error", err)
	}

	if closeWhenDone && h.weave != nil && callID != "" {
		h.traceEndChat(context.Background(), callID, startDone, time.Now(), fullResponse)
	}
}

// proxyResponse handles non-streaming responses
func (h *Handler) proxyResponse(w http.ResponseWriter, respBody io.Reader, callID string, startDone <-chan struct{}) {
	body, err := io.ReadAll(respBody)
	if err != nil {
		slog.Error("error reading upstream response", "error", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(body)

	if h.weave != nil && callID != "" {
		var resp ChatCompletion
		if err := json.Unmarshal(body, &resp); err == nil && len(resp.Choices) > 0 {
			h.traceEndChat(context.Background(), callID, startDone, time.Now(), resp.Choices[0].Message.Content)
		} else {
			h.traceEndChat(context.Background(), callID, startDone, time.Now(), "")
		}
	}
}
