package gh

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// RunsOpts filters ListRuns. Zero values are omitted.
type RunsOpts struct {
	Branch  string
	Status  string
	PerPage int
}

// ListRuns returns the newest runs of a repo. When etag matches, the
// response is NotModified and runs is nil.
func (c *Client) ListRuns(ctx context.Context, owner, repo string, opts RunsOpts, etag string) ([]Run, Response, error) {
	q := url.Values{}
	if opts.Branch != "" {
		q.Set("branch", opts.Branch)
	}
	if opts.Status != "" {
		q.Set("status", opts.Status)
	}
	if opts.PerPage > 0 {
		q.Set("per_page", strconv.Itoa(opts.PerPage))
	}
	path := fmt.Sprintf("/repos/%s/%s/actions/runs", owner, repo)
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var body struct {
		WorkflowRuns []Run `json:"workflow_runs"`
	}
	resp, err := c.get(ctx, path, etag, &body)
	if err != nil || resp.NotModified {
		return nil, resp, err
	}
	return body.WorkflowRuns, resp, nil
}

// GetRun returns a single run.
func (c *Client) GetRun(ctx context.Context, owner, repo string, id int64) (Run, error) {
	var r Run
	_, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/actions/runs/%d", owner, repo, id), "", &r)
	return r, err
}
