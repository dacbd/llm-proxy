package openai

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"

	"github.com/dacbd/llm-proxy/weave"
)

// hashString returns a xxhash64 hash of the input string as a hexadecimal string.
func hashString(s string) string {
	h := xxhash.New()
	h.Write([]byte(s))
	return strconv.FormatUint(h.Sum64(), 16)
}

// ensureParentSpanExists checks if a parent span exists in Weave and creates it if not.
// This runs in a background goroutine to avoid blocking the request.
func (h *Handler) ensureParentSpanExists(ctx context.Context, parentID string, firstMsg Message) {
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
			OpName:    "openai.chat",
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
func (h *Handler) traceStartChat(ctx context.Context, callID string, parentID *string, startedAt time.Time, req ChatCompletionRequest) <-chan struct{} {
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

	if req.MaxTokens != nil {
		inputs["max_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		inputs["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		inputs["top_p"] = *req.TopP
	}

	go func() {
		defer close(done)
		var traceID *string
		if parentID == nil {
			// For root spans, generate a trace ID
			traceIDStr := uuid.New().String()
			traceID = &traceIDStr
		}

		_, err := h.weave.CallStart(ctx, weave.StartedCallSchemaForInsert{
			ID:         &callID,
			OpName:     "openai.chat.completions",
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
func (h *Handler) traceEndChat(ctx context.Context, callID string, startDone <-chan struct{}, endedAt time.Time, fullResponse string) {
	if h.weave == nil {
		return
	}

	output, _ := json.Marshal(map[string]any{
		"response": fullResponse,
	})

	// Since we don't have exact token counts from OpenAI responses in streaming mode,
	// we estimate or leave as 0
	one := 1

	go func() {
		if startDone != nil {
			<-startDone // ensure CallStart has been accepted before ending
		}
		err := h.weave.CallEnd(ctx, weave.EndedCallSchemaForInsert{
			ID:      callID,
			EndedAt: endedAt,
			Output:  json.RawMessage(output),
			Summary: weave.SummaryInsertMap{
				Usage: map[string]weave.LLMUsageSchema{
					"openai": {
						Requests: &one,
					},
				},
			},
		})
		if err != nil {
			slog.Warn("weave CallEnd failed for chat", "error", err)
		}
	}()
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
	// Standard headers that OpenAI accepts
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
