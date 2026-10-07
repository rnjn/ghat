package gh

import (
	"context"
	"time"
)

// ListRepos returns the user's repos pushed at or after pushedSince, newest
// first. It pages through /user/repos sorted by push time and stops at the
// first repo older than the cutoff.
func (c *Client) ListRepos(ctx context.Context, pushedSince time.Time) ([]Repo, error) {
	var out []Repo
	next := "/user/repos?sort=pushed&per_page=100"
	for next != "" {
		var page []Repo
		resp, err := c.get(ctx, next, "", &page)
		if err != nil {
			return nil, err
		}
		for _, r := range page {
			if r.PushedAt.Before(pushedSince) {
				return out, nil
			}
			out = append(out, r)
		}
		next = resp.NextPage
	}
	return out, nil
}
