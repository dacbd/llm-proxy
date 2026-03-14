package weave

import (
	"context"
	"iter"
)

// CallStart begins a new call. Returns the assigned call ID and trace ID.
func (c *Client) CallStart(ctx context.Context, start StartedCallSchemaForInsert) (CallStartRes, error) {
	start.ProjectID = c.projectID
	var res CallStartRes
	if err := c.post(ctx, "/call/start", CallStartReq{Start: start}, &res); err != nil {
		return CallStartRes{}, err
	}
	return res, nil
}

// CallEnd completes a call previously started with CallStart.
func (c *Client) CallEnd(ctx context.Context, end EndedCallSchemaForInsert) error {
	end.ProjectID = c.projectID
	return c.post(ctx, "/call/end", CallEndReq{End: end}, nil)
}

// CallRead fetches a single call by ID.
func (c *Client) CallRead(ctx context.Context, callID string) (*CallSchema, error) {
	var res CallReadRes
	if err := c.post(ctx, "/call/read", CallReadReq{
		ProjectID: c.projectID,
		ID:        callID,
	}, &res); err != nil {
		return nil, err
	}
	return res.Call, nil
}

// CallUpdate updates mutable fields on an existing call (e.g. display name).
func (c *Client) CallUpdate(ctx context.Context, req CallUpdateReq) error {
	req.ProjectID = c.projectID
	return c.post(ctx, "/call/update", req, nil)
}

// CallsDelete deletes calls by ID. Returns the number of calls deleted.
func (c *Client) CallsDelete(ctx context.Context, callIDs []string) (int, error) {
	var res CallsDeleteRes
	if err := c.post(ctx, "/calls/delete", CallsDeleteReq{
		ProjectID: c.projectID,
		CallIDs:   callIDs,
	}, &res); err != nil {
		return 0, err
	}
	return res.NumDeleted, nil
}

// CallsQuery streams calls matching the given query. The returned iterator
// yields one *CallSchema per line until the stream is exhausted or the
// context is cancelled.
//
//	for call, err := range client.CallsQuery(ctx, req) {
//	    if err != nil { ... }
//	    // use call
//	}
func (c *Client) CallsQuery(ctx context.Context, req CallsQueryReq) iter.Seq2[*CallSchema, error] {
	req.ProjectID = c.projectID
	return c.streamQuery(ctx, "/calls/stream_query", req)
}
