package gh

import (
	"context"
	"fmt"
)

// ListJobs returns every job of the latest attempt of a run.
func (c *Client) ListJobs(ctx context.Context, owner, repo string, runID int64) ([]Job, error) {
	var out []Job
	next := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs?per_page=100", owner, repo, runID)
	for next != "" {
		var body struct {
			Jobs []Job `json:"jobs"`
		}
		resp, err := c.get(ctx, next, "", &body)
		if err != nil {
			return nil, err
		}
		out = append(out, body.Jobs...)
		next = resp.NextPage
	}
	return out, nil
}

// GetJob returns a single job with its steps.
func (c *Client) GetJob(ctx context.Context, owner, repo string, id int64) (Job, error) {
	var j Job
	_, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/actions/jobs/%d", owner, repo, id), "", &j)
	return j, err
}
