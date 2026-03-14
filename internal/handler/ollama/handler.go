package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	ollamaapi "github.com/ollama/ollama/api"

	"github.com/dacbd/llm-proxy/weave"
)

type Handler struct {
	upstreamURL string
	client      *http.Client
	weave       weave.API // nil if tracing disabled
}

func NewHandler(upstreamURL string, weaveClient weave.API) *Handler {
	return &Handler{
		upstreamURL: upstreamURL,
		client:      &http.Client{},
		weave:       weaveClient,
	}
}

func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	// Read body into memory for forwarding and parsing
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("failed to read request body", "error", err)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Forward to upstream immediately (non-blocking for parsing)
	resp, err := h.forwardGenerateRequest(r.Context(), r, body)
	if err != nil {
		slog.Error("upstream request failed", "error", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Parse body separately for tracing (doesn't block upstream)
	var req ollamaapi.GenerateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		slog.Error("failed to parse request body", "error", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	slog.Info("generate request", "model", req.Model, "prompt_len", len(req.Prompt))

	startedAt := time.Now()
	callID := uuid.New().String()
	startDone := h.traceStart(callID, startedAt, req)

	copyResponseHeaders(w, resp)

	// Stream the response
	genResp, err := h.streamGenerateResponse(w, resp.Body, callID, startDone)
	if err != nil {
		slog.Error("error streaming response", "error", err)
	}

	slog.Info("generate complete",
		"model", genResp.Model,
		"eval_count", genResp.EvalCount,
		"eval_duration", genResp.EvalDuration,
		"prompt_eval_count", genResp.PromptEvalCount,
	)
}

func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	var req ollamaapi.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	slog.Info("chat request", "model", req.Model, "messages_count", len(req.Messages))

	// Handle Weave tracing if enabled
	var (
		startedAt    = time.Now()
		callID       string
		parentID     *string
		startDone    <-chan struct{}
		fullResponse string
	)

	if h.weave != nil && len(req.Messages) > 0 {
		// Generate span ID from latest message (last user or assistant message)
		latestMsg := req.Messages[len(req.Messages)-1]
		callID = hashString(latestMsg.Content)

		// Determine parent ID from first message (if exists)
		if len(req.Messages) > 1 {
			firstMsg := req.Messages[0]
			parentIDStr := hashString(firstMsg.Content)
			parentID = &parentIDStr

			// Check if parent span exists, create if not
			go h.ensureParentSpanExists(context.Background(), *parentID, firstMsg)
		}

		// Start the current span
		startDone = h.traceStartChat(context.Background(), callID, parentID, startedAt, req)
	}

	body, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.upstreamURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
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
	isStreaming := req.Stream == nil || *req.Stream

	if isStreaming {
		// Handle streaming response
		flusher, canFlush := w.(http.Flusher)
		scanner := bufio.NewScanner(resp.Body)

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var chatResp ollamaapi.ChatResponse
			if err := json.Unmarshal(line, &chatResp); err != nil {
				slog.Error("failed to parse upstream response line", "error", err)
				w.Write(line)
				w.Write([]byte("\n"))
				if canFlush {
					flusher.Flush()
				}
				continue
			}

			// Accumulate response content
			fullResponse += chatResp.Message.Content

			w.Write(line)
			w.Write([]byte("\n"))
			if canFlush {
				flusher.Flush()
			}

			if chatResp.Done {
				slog.Info("chat complete",
					"model", chatResp.Model,
					"eval_count", chatResp.Metrics.EvalCount,
					"eval_duration", chatResp.Metrics.EvalDuration,
					"prompt_eval_count", chatResp.Metrics.PromptEvalCount,
				)
				// End the span
				if h.weave != nil {
					h.traceEndChat(context.Background(), callID, startDone, time.Now(), fullResponse, chatResp)
				}
				break
			}
		}

		if err := scanner.Err(); err != nil {
			slog.Error("error reading upstream response", "error", err)
		}
	} else {
		// Handle non-streaming response
		var chatResp ollamaapi.ChatResponse
		if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
			slog.Error("failed to decode non-streaming response", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Write response to client
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(chatResp); err != nil {
			slog.Error("error encoding response", "error", err)
		}

		// End the span for non-streaming response
		if h.weave != nil {
			fullResponse = chatResp.Message.Content
			h.traceEndChat(context.Background(), callID, startDone, time.Now(), fullResponse, chatResp)
		}
	}
}

// List proxies the /api/tags request to upstream Ollama server
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	slog.Info("list request")

	// Forward to upstream
	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodGet, h.upstreamURL+"/api/tags", nil)
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

	// Copy response body
	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Error("error copying upstream response", "error", err)
	}
}

// ListRunning proxies the /api/ps request to upstream Ollama server
func (h *Handler) ListRunning(w http.ResponseWriter, r *http.Request) {
	slog.Info("list running request")

	// Forward to upstream
	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodGet, h.upstreamURL+"/api/ps", nil)
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

	// Copy response body
	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Error("error copying upstream response", "error", err)
	}
}
