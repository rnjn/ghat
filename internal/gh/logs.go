package gh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrLogNotReady means GitHub has no log to serve yet: either the API
// answers 404, or it redirects to a log blob that does not exist yet (the
// current results backend only writes the job log once the job completes).
// For an in-progress job this is expected, not a failure.
var ErrLogNotReady = errors.New("job log not available yet")

// JobLog returns the plain-text log of a job: every completed step so far.
// GitHub answers with a 302 to a short-lived signed URL; the redirect is
// followed without the API token, and the URL is never cached.
func (c *Client) JobLog(ctx context.Context, owner, repo string, jobID int64) ([]byte, error) {
	req, err := c.newRequest(ctx, fmt.Sprintf("/repos/%s/%s/actions/jobs/%d/logs", owner, repo, jobID))
	if err != nil {
		return nil, err
	}
	noFollow := *c.http
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := noFollow.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	resp := c.parseResponse(res)
	switch {
	case res.StatusCode == http.StatusNotFound:
		return nil, ErrLogNotReady
	case res.StatusCode >= 300 && res.StatusCode < 400:
		loc, err := res.Location()
		if err != nil {
			return nil, fmt.Errorf("log redirect: %w", err)
		}
		return c.fetchBlob(ctx, loc.String())
	}
	if err := checkStatus(res, resp); err != nil {
		return nil, err
	}
	return io.ReadAll(res.Body)
}

// blobTimeout bounds a whole log download; large logs on slow links take
// longer than the API client's per-request timeout.
const blobTimeout = 10 * time.Minute

// fetchBlob downloads a signed log URL with no GitHub credentials attached.
func (c *Client) fetchBlob(ctx context.Context, u string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, blobTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	blob := &http.Client{Transport: c.http.Transport}
	res, err := blob.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusNotFound {
		return nil, ErrLogNotReady
	}
	if res.StatusCode != http.StatusOK {
		return nil, &APIError{Status: res.StatusCode, Message: "log download failed"}
	}
	return io.ReadAll(res.Body)
}
