package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"
	ollamaapi "github.com/ollama/ollama/api"

	"github.com/dacbd/llm-proxy/weave"
)

// hashString returns a xxhash64 hash of the input string as a hexadecimal string.
func hashString(s string) string {
	h := xxhash.New()
	h.Write([]byte(s))
	return strconv.FormatUint(h.Sum64(), 16)
}

// getMessageHash returns a hash of a message's content for use as a span ID.
func getMessageHash(msg ollamaapi.Message) string {
	if msg.Content == "" {
		return hashString("empty")
	}
	return hashString(msg.Content)
}

// ensureParentSpanExists checks if a parent span exists in Weave and creates it if not.
// This runs in a background goroutine to avoid blocking the request.
func (h *Handler) ensureParentSpanExists(ctx context.Context, parentID string, firstMsg ollamaapi.Message) {
	if h.weave == nil {
		return
	}

	// Check if parent span exists
	existingCall, err := h.weave.CallRead(ctx, parentID)

	// If span doesn't exist (we get an error or nil call), create it
	if err != nil || existingCall == nil || existingCall.ID == "" {
		// Create the parent span
		startedAt := time.Now()
		_, err := h.weave.CallStart(ctx, weave.StartedCallSchemaForInsert{
			ID:        &parentID,
			OpName:    "ollama.chat",
			StartedAt: startedAt,
			Attributes: map[string]any{
				"model": firstMsg.Content[:min(50, len(firstMsg.Content))], // Truncate for display
			},
			Inputs: map[string]any{
				"first_message": firstMsg.Content,
				"role":          firstMsg.Role,
			},
		})

		if err != nil {
			slog.Warn("failed to create parent span in weave", "error", err, "parent_id", parentID)
		} else {
			slog.Debug("created parent span in weave", "parent_id", parentID)
		}
	} else {
		slog.Debug("parent span already exists in weave", "parent_id", parentID)
	}
}

// traceStartChat fires CallStart in a background goroutine and returns a channel
// that is closed when the start request completes.
func (h *Handler) traceStartChat(ctx context.Context, callID string, parentID *string, startedAt time.Time, req ollamaapi.ChatRequest) <-chan struct{} {
	done := make(chan struct{})
	if h.weave == nil {
		close(done)
		return done
	}

	// Prepare inputs from the chat request
	var messagesInput []map[string]string
	for _, msg := range req.Messages {
		messagesInput = append(messagesInput, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	inputs := map[string]any{
		"model":    req.Model,
		"messages": messagesInput,
		"stream":   req.Stream,
	}

	if req.Format != nil {
		inputs["format"] = req.Format
	}
	if req.Options != nil {
		inputs["options"] = req.Options
	}
	if req.Think != nil {
		inputs["think"] = req.Think
	}

	go func() {
		defer close(done)
		var traceID *string
		if parentID == nil {
			// For root spans, generate a trace ID
			traceIDStr := uuid.New().String()
			traceID = &traceIDStr
		}
		// For child spans, we don't specify traceID - it should be inherited from parent

		_, err := h.weave.CallStart(ctx, weave.StartedCallSchemaForInsert{
			ID:         &callID,
			OpName:     "ollama.chat",
			TraceID:    traceID,
			ParentID:   parentID,
			StartedAt:  startedAt,
			Attributes: map[string]any{"model": req.Model},
			Inputs:     inputs,
		})
		if err != nil {
			slog.Warn("weave CallStart failed for chat", "error", err)
		}
	}()

	return done
}

// traceEndChat waits for CallStart to finish then fires CallEnd in a background goroutine.
func (h *Handler) traceEndChat(ctx context.Context, callID string, startDone <-chan struct{}, endedAt time.Time, fullResponse string, final ollamaapi.ChatResponse) {
	if h.weave == nil {
		return
	}

	output, _ := json.Marshal(map[string]any{
		"model":    final.Model,
		"response": fullResponse,
		// Include thinking if present
		"thinking": final.Message.Thinking,
	})

	promptTokens := final.Metrics.PromptEvalCount
	completionTokens := final.Metrics.EvalCount
	totalTokens := promptTokens + completionTokens
	one := 1

	go func() {
		<-startDone // ensure CallStart has been accepted before ending
		err := h.weave.CallEnd(ctx, weave.EndedCallSchemaForInsert{
			ID:      callID,
			EndedAt: endedAt,
			Output:  json.RawMessage(output),
			Summary: weave.SummaryInsertMap{
				Usage: map[string]weave.LLMUsageSchema{
					final.Model: {
						PromptTokens:     &promptTokens,
						CompletionTokens: &completionTokens,
						TotalTokens:      &totalTokens,
						Requests:         &one,
					},
				},
			},
		})
		if err != nil {
			slog.Warn("weave CallEnd failed for chat", "error", err)
		}
	}()
}

// forwardGenerateRequest sends the request to the upstream Ollama server
func (h *Handler) forwardGenerateRequest(ctx context.Context, r *http.Request, body []byte) (*http.Response, error) {
	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, h.upstreamURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	upstream.Header.Set("Content-Type", "application/json")
	CopyRequestHeaders(upstream, r)
	return h.client.Do(upstream)
}

// copyResponseHeaders copies headers from upstream response to client response
func copyResponseHeaders(w http.ResponseWriter, resp *http.Response) {
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
}

// CopyRequestHeaders copies relevant headers from incoming request to upstream request
func CopyRequestHeaders(upstream *http.Request, r *http.Request) {
	// Standard headers that Ollama accepts
	standardHeaders := []string{
		"Authorization",
		"Content-Type",
		"User-Agent",
		"Accept",
		"X-Requested-With",
		"X-Request-Id",
		"X-Correlation-Id",
	}
	for _, header := range standardHeaders {
		if v := r.Header.Get(header); v != "" {
			upstream.Header.Set(header, v)
		}
	}

	// OpenAI compatibility headers
	for header, values := range r.Header {
		lowerHeader := strings.ToLower(header)
		if strings.HasPrefix(lowerHeader, "openai-") || strings.HasPrefix(lowerHeader, "x-stainless-") {
			for _, v := range values {
				upstream.Header.Add(header, v)
			}
		}
	}
}

// streamGenerateResponse handles streaming the response from upstream to the client
func (h *Handler) streamGenerateResponse(w http.ResponseWriter, respBody io.Reader, callID string, startDone <-chan struct{}) (ollamaapi.GenerateResponse, error) {
	var fullResponse string
	var finalResp ollamaapi.GenerateResponse
	flusher, canFlush := w.(http.Flusher)
	scanner := bufio.NewScanner(respBody)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var genResp ollamaapi.GenerateResponse
		if err := json.Unmarshal(line, &genResp); err != nil {
			slog.Error("failed to parse upstream response line", "error", err)
			w.Write(line)
			w.Write([]byte("\n"))
			if canFlush {
				flusher.Flush()
			}
			continue
		}

		fullResponse += genResp.Response
		finalResp = genResp

		w.Write(line)
		w.Write([]byte("\n"))
		if canFlush {
			flusher.Flush()
		}

		if genResp.Done {
			h.traceEnd(callID, startDone, time.Now(), fullResponse, genResp)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return finalResp, err
	}

	return finalResp, nil
}

// traceStart fires CallStart in a background goroutine and returns a channel
// that is closed when the start request completes.
func (h *Handler) traceStart(callID string, startedAt time.Time, req ollamaapi.GenerateRequest) <-chan struct{} {
	done := make(chan struct{})
	if h.weave == nil {
		close(done)
		return done
	}

	inputs := map[string]any{
		"model":  req.Model,
		"prompt": req.Prompt,
	}
	if req.System != "" {
		inputs["system"] = req.System
	}

	go func() {
		defer close(done)
		_, err := h.weave.CallStart(context.Background(), weave.StartedCallSchemaForInsert{
			ID:         &callID,
			OpName:     "ollama.generate",
			StartedAt:  startedAt,
			Attributes: map[string]any{"model": req.Model},
			Inputs:     inputs,
		})
		if err != nil {
			slog.Warn("weave CallStart failed", "error", err)
		}
	}()

	return done
}

// traceEnd waits for CallStart to finish then fires CallEnd in a background goroutine.
func (h *Handler) traceEnd(callID string, startDone <-chan struct{}, endedAt time.Time, fullResponse string, final ollamaapi.GenerateResponse) {
	if h.weave == nil {
		return
	}

	output, _ := json.Marshal(map[string]any{
		"model":    final.Model,
		"response": fullResponse,
	})

	promptTokens := final.PromptEvalCount
	completionTokens := final.EvalCount
	totalTokens := promptTokens + completionTokens
	one := 1

	go func() {
		<-startDone // ensure CallStart has been accepted before ending
		err := h.weave.CallEnd(context.Background(), weave.EndedCallSchemaForInsert{
			ID:      callID,
			EndedAt: endedAt,
			Output:  json.RawMessage(output),
			Summary: weave.SummaryInsertMap{
				Usage: map[string]weave.LLMUsageSchema{
					final.Model: {
						PromptTokens:     &promptTokens,
						CompletionTokens: &completionTokens,
						TotalTokens:      &totalTokens,
						Requests:         &one,
					},
				},
			},
		})
		if err != nil {
			slog.Warn("weave CallEnd failed", "error", err)
		}
	}()
}
