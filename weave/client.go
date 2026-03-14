package weave

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const defaultBaseURL = "https://trace.wandb.ai"

var _ API = (*Client)(nil)

// Client is a Weave service API client.
type Client struct {
	projectID  string // "entity/project"
	baseURL    string
	authHeader string
	http       *http.Client
}

// NewClient creates a Weave client. project is the full W&B project
// identifier in "entity/project" format, matching WANDB_PROJECT.
func NewClient(apiKey, project string) *Client {
	encoded := base64.StdEncoding.EncodeToString([]byte("api:" + apiKey))
	return &Client{
		projectID:  project,
		baseURL:    defaultBaseURL,
		authHeader: "Basic " + encoded,
		http:       &http.Client{},
	}
}

// ProjectID returns the project ID used by this client.
func (c *Client) ProjectID() string {
	return c.projectID
}

// post sends a JSON POST request and decodes the response into resp.
func (c *Client) post(ctx context.Context, path string, req, resp any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("weave: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("weave: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", c.authHeader)

	slog.Debug("weave request", "path", path)

	start := time.Now()
	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		slog.Error("weave request failed", "path", path, "error", err)
		return fmt.Errorf("weave: http: %w", err)
	}
	defer httpResp.Body.Close()

	elapsed := time.Since(start)

	if httpResp.StatusCode >= 400 {
		raw, _ := io.ReadAll(httpResp.Body)
		body := strings.TrimSpace(string(raw))
		slog.Error("weave error response", "path", path, "status", httpResp.StatusCode, "body", body, "duration", elapsed)
		return fmt.Errorf("weave: http %d: %s", httpResp.StatusCode, body)
	}

	slog.Debug("weave response", "path", path, "status", httpResp.StatusCode, "duration", elapsed)

	if resp != nil {
		if err := json.NewDecoder(httpResp.Body).Decode(resp); err != nil {
			return fmt.Errorf("weave: decode response: %w", err)
		}
	}
	return nil
}

// streamQuery sends a POST and returns an iterator over JSONL response lines.
func (c *Client) streamQuery(ctx context.Context, path string, req any) iter.Seq2[*CallSchema, error] {
	return func(yield func(*CallSchema, error) bool) {
		body, err := json.Marshal(req)
		if err != nil {
			yield(nil, fmt.Errorf("weave: marshal request: %w", err))
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			yield(nil, fmt.Errorf("weave: build request: %w", err))
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/jsonl")
		httpReq.Header.Set("Authorization", c.authHeader)

		slog.Debug("weave stream request", "path", path)

		start := time.Now()
		httpResp, err := c.http.Do(httpReq)
		if err != nil {
			slog.Error("weave stream request failed", "path", path, "error", err)
			yield(nil, fmt.Errorf("weave: http: %w", err))
			return
		}
		defer httpResp.Body.Close()

		if httpResp.StatusCode >= 400 {
			raw, _ := io.ReadAll(httpResp.Body)
			body := strings.TrimSpace(string(raw))
			slog.Error("weave stream error response", "path", path, "status", httpResp.StatusCode, "body", body)
			yield(nil, fmt.Errorf("weave: http %d: %s", httpResp.StatusCode, body))
			return
		}

		slog.Debug("weave stream response", "path", path, "status", httpResp.StatusCode)

		var count int
		scanner := bufio.NewScanner(httpResp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var call CallSchema
			if err := json.Unmarshal(line, &call); err != nil {
				slog.Warn("weave stream decode error", "path", path, "error", err)
				if !yield(nil, fmt.Errorf("weave: decode line: %w", err)) {
					return
				}
				continue
			}
			count++
			if !yield(&call, nil) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			slog.Error("weave stream read error", "path", path, "error", err)
			yield(nil, fmt.Errorf("weave: read stream: %w", err))
			return
		}
		slog.Debug("weave stream complete", "path", path, "count", count, "duration", time.Since(start))
	}
}
