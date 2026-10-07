package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// post sends a JSON body (nil for none) and treats any 2xx as success.
// 403 and 404 become *PermissionError, since on a write they mean the
// token cannot act here.
func (c *Client) post(ctx context.Context, path string, body any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := c.newRequestMethod(ctx, http.MethodPost, path, &buf)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	err = checkStatus(res, c.parseResponse(res))
	var ae *APIError
	if errors.As(err, &ae) && (ae.Status == http.StatusForbidden || ae.Status == http.StatusNotFound) {
		return &PermissionError{Status: ae.Status, Message: ae.Message}
	}
	return err
}

// RerunRun reruns every job of a run.
func (c *Client) RerunRun(ctx context.Context, owner, repo string, runID int64) error {
	return c.post(ctx, fmt.Sprintf("/repos/%s/%s/actions/runs/%d/rerun", owner, repo, runID), nil)
}

// RerunFailedJobs reruns only the failed jobs of a run.
func (c *Client) RerunFailedJobs(ctx context.Context, owner, repo string, runID int64) error {
	return c.post(ctx, fmt.Sprintf("/repos/%s/%s/actions/runs/%d/rerun-failed-jobs", owner, repo, runID), nil)
}

// CancelRun cancels a queued or running run.
func (c *Client) CancelRun(ctx context.Context, owner, repo string, runID int64) error {
	return c.post(ctx, fmt.Sprintf("/repos/%s/%s/actions/runs/%d/cancel", owner, repo, runID), nil)
}

// DispatchWorkflow triggers a workflow_dispatch event on ref.
func (c *Client) DispatchWorkflow(ctx context.Context, owner, repo string, workflowID int64, ref string, inputs map[string]string) error {
	body := struct {
		Ref    string            `json:"ref"`
		Inputs map[string]string `json:"inputs,omitempty"`
	}{Ref: ref, Inputs: inputs}
	return c.post(ctx, fmt.Sprintf("/repos/%s/%s/actions/workflows/%d/dispatches", owner, repo, workflowID), body)
}
