package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.github.com"
	apiVersion     = "2022-11-28"
)

// Client is a typed GitHub REST client. It holds no polling logic.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at a different API root (used by tests).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient replaces the underlying *http.Client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// New returns a client authenticating with token.
func New(token string, opts ...Option) *Client {
	c := &Client{baseURL: defaultBaseURL, token: token, http: &http.Client{Timeout: 30 * time.Second}}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Response carries the metadata of a GET that callers need for caching,
// rate-limit tracking and pagination.
type Response struct {
	ETag          string
	NotModified   bool
	RateRemaining int
	RateReset     time.Time
	NextPage      string
}

func (c *Client) url(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return c.baseURL + path
}

func (c *Client) newRequest(ctx context.Context, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	return req, nil
}

// get fetches path (relative to the base URL, or absolute for pagination
// links) and decodes JSON into out. When etag is set it is sent as
// If-None-Match; a 304 returns NotModified and leaves out untouched.
func (c *Client) get(ctx context.Context, path, etag string, out any) (Response, error) {
	req, err := c.newRequest(ctx, path)
	if err != nil {
		return Response{}, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer func() { _ = res.Body.Close() }()

	resp := parseResponse(res)
	if res.StatusCode == http.StatusNotModified {
		resp.NotModified = true
		if resp.ETag == "" {
			resp.ETag = etag
		}
		return resp, nil
	}
	if err := checkStatus(res, resp); err != nil {
		return resp, err
	}
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			return resp, fmt.Errorf("decode %s: %w", req.URL.Path, err)
		}
	}
	return resp, nil
}

func parseResponse(res *http.Response) Response {
	resp := Response{ETag: res.Header.Get("ETag"), RateRemaining: -1}
	if v, err := strconv.Atoi(res.Header.Get("X-RateLimit-Remaining")); err == nil {
		resp.RateRemaining = v
	}
	if v, err := strconv.ParseInt(res.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		resp.RateReset = time.Unix(v, 0)
	}
	resp.NextPage = nextLink(res.Header.Get("Link"))
	return resp
}

// checkStatus turns a non-2xx response into a *RateLimitError or *APIError.
func checkStatus(res *http.Response, resp Response) error {
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	if (res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusTooManyRequests) && resp.RateRemaining == 0 {
		return &RateLimitError{Reset: resp.RateReset}
	}
	var body struct {
		Message string `json:"message"`
	}
	data, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	_ = json.Unmarshal(data, &body)
	return &APIError{Status: res.StatusCode, Message: body.Message}
}

var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func nextLink(h string) string {
	if m := linkNextRe.FindStringSubmatch(h); m != nil {
		return m[1]
	}
	return ""
}
