package weave

import (
	"context"
	"iter"
)

// API is the interface for interacting with the Weave service.
// Use Client for production and a mock for tests.
type API interface {
	ProjectID() string
	CallStart(ctx context.Context, start StartedCallSchemaForInsert) (CallStartRes, error)
	CallEnd(ctx context.Context, end EndedCallSchemaForInsert) error
	CallRead(ctx context.Context, callID string) (*CallSchema, error)
	CallUpdate(ctx context.Context, req CallUpdateReq) error
	CallsDelete(ctx context.Context, callIDs []string) (int, error)
	CallsQuery(ctx context.Context, req CallsQueryReq) iter.Seq2[*CallSchema, error]
}
