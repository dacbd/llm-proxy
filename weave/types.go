package weave

import (
	"encoding/json"
	"time"
)

// StartedCallSchemaForInsert is the payload for starting a call.
type StartedCallSchemaForInsert struct {
	ProjectID   string         `json:"project_id"`
	OpName      string         `json:"op_name"`
	StartedAt   time.Time      `json:"started_at"`
	Attributes  map[string]any `json:"attributes"`
	Inputs      map[string]any `json:"inputs"`
	ID          *string        `json:"id,omitempty"`
	DisplayName *string        `json:"display_name,omitempty"`
	TraceID     *string        `json:"trace_id,omitempty"`
	ParentID    *string        `json:"parent_id,omitempty"`
	WBRunID     *string        `json:"wb_run_id,omitempty"`
}

// EndedCallSchemaForInsert is the payload for ending a call.
type EndedCallSchemaForInsert struct {
	ProjectID string           `json:"project_id"`
	ID        string           `json:"id"`
	EndedAt   time.Time        `json:"ended_at"`
	Summary   SummaryInsertMap `json:"summary"`
	Output    json.RawMessage  `json:"output,omitempty"`
	Exception *string          `json:"exception,omitempty"`
}

// SummaryInsertMap holds aggregated metrics for a call.
type SummaryInsertMap struct {
	Usage map[string]LLMUsageSchema `json:"usage,omitempty"`
}

// LLMUsageSchema holds token counts for an LLM call.
type LLMUsageSchema struct {
	PromptTokens     *int `json:"prompt_tokens,omitempty"`
	InputTokens      *int `json:"input_tokens,omitempty"`
	CompletionTokens *int `json:"completion_tokens,omitempty"`
	OutputTokens     *int `json:"output_tokens,omitempty"`
	TotalTokens      *int `json:"total_tokens,omitempty"`
	Requests         *int `json:"requests,omitempty"`
}

// CallSchema is a fully hydrated call as returned by the API.
type CallSchema struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	OpName      string          `json:"op_name"`
	TraceID     string          `json:"trace_id"`
	StartedAt   time.Time       `json:"started_at"`
	Attributes  map[string]any  `json:"attributes"`
	Inputs      map[string]any  `json:"inputs"`
	DisplayName *string         `json:"display_name,omitempty"`
	ParentID    *string         `json:"parent_id,omitempty"`
	EndedAt     *time.Time      `json:"ended_at,omitempty"`
	Exception   *string         `json:"exception,omitempty"`
	Output      json.RawMessage `json:"output,omitempty"`
	Summary     map[string]any  `json:"summary,omitempty"`
	WBUserID    *string         `json:"wb_user_id,omitempty"`
	WBRunID     *string         `json:"wb_run_id,omitempty"`
	DeletedAt   *time.Time      `json:"deleted_at,omitempty"`
}

// --- Request / Response wrappers ---

type CallStartReq struct {
	Start StartedCallSchemaForInsert `json:"start"`
}

type CallStartRes struct {
	ID      string `json:"id"`
	TraceID string `json:"trace_id"`
}

type CallEndReq struct {
	End EndedCallSchemaForInsert `json:"end"`
}

type CallEndRes struct{}

type CallReadReq struct {
	ProjectID string `json:"project_id"`
	ID        string `json:"id"`
}

type CallReadRes struct {
	Call *CallSchema `json:"call"`
}

type CallUpdateReq struct {
	ProjectID   string  `json:"project_id"`
	CallID      string  `json:"call_id"`
	DisplayName *string `json:"display_name,omitempty"`
}

type CallUpdateRes struct{}

type CallsDeleteReq struct {
	ProjectID string   `json:"project_id"`
	CallIDs   []string `json:"call_ids"`
}

type CallsDeleteRes struct {
	NumDeleted int `json:"num_deleted"`
}

// --- Stream query ---

type CallsQueryReq struct {
	ProjectID       string       `json:"project_id"`
	Filter          *CallsFilter `json:"filter,omitempty"`
	Limit           *int         `json:"limit,omitempty"`
	Offset          *int         `json:"offset,omitempty"`
	SortBy          []SortBy     `json:"sort_by,omitempty"`
	Columns         []string     `json:"columns,omitempty"`
	IncludeCosts    *bool        `json:"include_costs,omitempty"`
	IncludeFeedback *bool        `json:"include_feedback,omitempty"`
}

type CallsFilter struct {
	OpNames    []string `json:"op_names,omitempty"`
	ParentIDs  []string `json:"parent_ids,omitempty"`
	TraceIDs   []string `json:"trace_ids,omitempty"`
	CallIDs    []string `json:"call_ids,omitempty"`
	TraceRoots *bool    `json:"trace_roots_only,omitempty"`
	WBUserIDs  []string `json:"wb_user_ids,omitempty"`
	WBRunIDs   []string `json:"wb_run_ids,omitempty"`
}

type SortBy struct {
	Field     string `json:"field"`
	Direction string `json:"direction"` // "asc" | "desc"
}

// --- Batch upsert ---

type BatchItem struct {
	Mode string          `json:"mode"` // "start" | "end"
	Req  json.RawMessage `json:"req"`
}

type CallUpsertBatchReq struct {
	Batch []BatchItem `json:"batch"`
}

type CallUpsertBatchRes struct {
	Res []json.RawMessage `json:"res"`
}
