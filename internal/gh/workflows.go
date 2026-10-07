package gh

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

// Workflow is a workflow definition in a repository.
type Workflow struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
}

// ListWorkflows returns every workflow of a repository.
func (c *Client) ListWorkflows(ctx context.Context, owner, repo string) ([]Workflow, error) {
	var out []Workflow
	next := fmt.Sprintf("/repos/%s/%s/actions/workflows?per_page=100", owner, repo)
	for next != "" {
		var body struct {
			Workflows []Workflow `json:"workflows"`
		}
		resp, err := c.get(ctx, next, "", &body)
		if err != nil {
			return nil, err
		}
		out = append(out, body.Workflows...)
		next = resp.NextPage
	}
	return out, nil
}

// WorkflowFile returns the contents of a file at ref (default branch when
// ref is empty).
func (c *Client) WorkflowFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	p := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, path)
	if ref != "" {
		p += "?ref=" + url.QueryEscape(ref)
	}
	var body struct {
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	if _, err := c.get(ctx, p, "", &body); err != nil {
		return nil, err
	}
	if body.Encoding != "base64" {
		return []byte(body.Content), nil
	}
	return base64.StdEncoding.DecodeString(strings.ReplaceAll(body.Content, "\n", ""))
}
