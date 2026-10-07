package gh

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New("tok", WithBaseURL(srv.URL))
}

func TestGetSendsHeadersAndDecodes(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
			t.Errorf("X-GitHub-Api-Version = %q", got)
		}
		if r.URL.Path != "/things" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("ETag", `W/"abc"`)
		_, _ = w.Write([]byte(`{"name":"x"}`))
	})
	var out struct{ Name string }
	resp, err := c.get(context.Background(), "/things", "", &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != "x" || resp.ETag != `W/"abc"` || resp.NotModified {
		t.Fatalf("out=%+v resp=%+v", out, resp)
	}
}

func TestGetNotModified(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("If-None-Match"); got != `"e1"` {
			t.Errorf("If-None-Match = %q", got)
		}
		w.Header().Set("ETag", `"e1"`)
		w.WriteHeader(http.StatusNotModified)
	})
	out := struct{ Name string }{Name: "keep"}
	resp, err := c.get(context.Background(), "/things", `"e1"`, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.NotModified || out.Name != "keep" || resp.ETag != `"e1"` {
		t.Fatalf("resp=%+v out=%+v", resp, out)
	}
}

func TestGetParsesRateLimitAndLink(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "4321")
		w.Header().Set("X-RateLimit-Reset", "1760000000")
		w.Header().Set("Link", `<https://api.github.com/user/repos?page=1>; rel="prev", <https://api.github.com/user/repos?page=3>; rel="next", <https://api.github.com/user/repos?page=9>; rel="last"`)
		_, _ = w.Write([]byte(`{}`))
	})
	resp, err := c.get(context.Background(), "/user/repos", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.RateRemaining != 4321 {
		t.Errorf("RateRemaining = %d", resp.RateRemaining)
	}
	if !resp.RateReset.Equal(time.Unix(1760000000, 0)) {
		t.Errorf("RateReset = %v", resp.RateReset)
	}
	if resp.NextPage != "https://api.github.com/user/repos?page=3" {
		t.Errorf("NextPage = %q", resp.NextPage)
	}
}

func TestGetFollowsAbsoluteURL(t *testing.T) {
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "page=2" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	srvURL = srv.URL
	c := New("tok", WithBaseURL(srvURL))
	if _, err := c.get(context.Background(), srvURL+"/user/repos?page=2", "", nil); err != nil {
		t.Fatal(err)
	}
}

func TestGetRateLimitError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1760000000")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	})
	_, err := c.get(context.Background(), "/x", "", nil)
	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("err = %T %v, want *RateLimitError", err, err)
	}
	if !rl.Reset.Equal(time.Unix(1760000000, 0)) {
		t.Fatalf("Reset = %v", rl.Reset)
	}
}

func TestGetAPIError(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusBadGateway} {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "10")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		})
		_, err := c.get(context.Background(), "/x", "", nil)
		var ae *APIError
		if !errors.As(err, &ae) {
			t.Fatalf("status %d: err = %T %v, want *APIError", status, err, err)
		}
		if ae.Status != status || ae.Message != "Not Found" {
			t.Fatalf("status %d: got %+v", status, ae)
		}
	}
}

func TestGetUnauthorizedPointsToLogin(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	})
	_, err := c.get(context.Background(), "/x", "", nil)
	if err == nil || !strings.Contains(err.Error(), "gh auth login") || !strings.Contains(err.Error(), "GH_TOKEN") {
		t.Fatalf("err = %v, want hint naming GH_TOKEN and gh auth login", err)
	}
}

func TestIsTransient(t *testing.T) {
	for err, want := range map[error]bool{
		&APIError{Status: 500}:         true,
		&APIError{Status: 502}:         true,
		errors.New("connection reset"): true,
		&APIError{Status: 404}:         false,
		&APIError{Status: 401}:         false,
		&RateLimitError{}:              false,
		context.Canceled:               false,
		context.DeadlineExceeded:       false,
		ErrLogNotReady:                 false,
	} {
		if got := IsTransient(err); got != want {
			t.Errorf("IsTransient(%v) = %v, want %v", err, got, want)
		}
	}
}

func TestRateLimitTracksLatestResponse(t *testing.T) {
	remaining := "4000"
	status := http.StatusOK
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", remaining)
		w.Header().Set("X-RateLimit-Reset", "1760000000")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{}`))
	})
	if rem, _ := c.RateLimit(); rem != -1 {
		t.Fatalf("before any request remaining = %d, want -1", rem)
	}
	if _, err := c.get(context.Background(), "/x", "", nil); err != nil {
		t.Fatal(err)
	}
	rem, reset := c.RateLimit()
	if rem != 4000 || !reset.Equal(time.Unix(1760000000, 0)) {
		t.Fatalf("got %d %v", rem, reset)
	}
	remaining, status = "12", http.StatusInternalServerError
	_, _ = c.get(context.Background(), "/x", "", nil)
	if rem, _ := c.RateLimit(); rem != 12 {
		t.Fatalf("after error response remaining = %d, want 12", rem)
	}
}

func TestRateLimitConcurrent(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "5")
		_, _ = w.Write([]byte(`{}`))
	})
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = c.get(context.Background(), "/x", "", nil)
			_, _ = c.RateLimit()
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

func TestSecondaryRateLimitIsRateLimitError(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"retry-after": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "4000")
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"You have exceeded a secondary rate limit."}`))
		},
		"message only": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "4000")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"You have exceeded a secondary rate limit."}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, h)
			before := time.Now()
			_, err := c.get(context.Background(), "/x", "", nil)
			var rl *RateLimitError
			if !errors.As(err, &rl) {
				t.Fatalf("err = %T %v, want *RateLimitError", err, err)
			}
			if !rl.Reset.After(before.Add(20 * time.Second)) {
				t.Fatalf("Reset = %v, want at least ~30s ahead", rl.Reset)
			}
		})
	}
}
