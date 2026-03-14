package weave

import (
	"context"
	"iter"
)

// MockAPI is a test double for API. Each field is a function that can be
// overridden per-test. Unset fields return zero values and no error.
type MockAPI struct {
	CallStartFn   func(ctx context.Context, start StartedCallSchemaForInsert) (CallStartRes, error)
	CallEndFn     func(ctx context.Context, end EndedCallSchemaForInsert) error
	CallReadFn    func(ctx context.Context, callID string) (*CallSchema, error)
	CallUpdateFn  func(ctx context.Context, req CallUpdateReq) error
	CallsDeleteFn func(ctx context.Context, callIDs []string) (int, error)
	CallsQueryFn  func(ctx context.Context, req CallsQueryReq) iter.Seq2[*CallSchema, error]
	ProjectIDVal  string
}

var _ API = (*MockAPI)(nil)

func (m *MockAPI) ProjectID() string {
	return m.ProjectIDVal
}

func (m *MockAPI) CallStart(ctx context.Context, start StartedCallSchemaForInsert) (CallStartRes, error) {
	if m.CallStartFn != nil {
		return m.CallStartFn(ctx, start)
	}
	return CallStartRes{ID: "mock-call-id", TraceID: "mock-trace-id"}, nil
}

func (m *MockAPI) CallEnd(ctx context.Context, end EndedCallSchemaForInsert) error {
	if m.CallEndFn != nil {
		return m.CallEndFn(ctx, end)
	}
	return nil
}

func (m *MockAPI) CallRead(ctx context.Context, callID string) (*CallSchema, error) {
	if m.CallReadFn != nil {
		return m.CallReadFn(ctx, callID)
	}
	return nil, nil
}

func (m *MockAPI) CallUpdate(ctx context.Context, req CallUpdateReq) error {
	if m.CallUpdateFn != nil {
		return m.CallUpdateFn(ctx, req)
	}
	return nil
}

func (m *MockAPI) CallsDelete(ctx context.Context, callIDs []string) (int, error) {
	if m.CallsDeleteFn != nil {
		return m.CallsDeleteFn(ctx, callIDs)
	}
	return 0, nil
}

func (m *MockAPI) CallsQuery(ctx context.Context, req CallsQueryReq) iter.Seq2[*CallSchema, error] {
	if m.CallsQueryFn != nil {
		return m.CallsQueryFn(ctx, req)
	}
	return func(yield func(*CallSchema, error) bool) {}
}
