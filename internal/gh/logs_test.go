package gh

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJobLogFollowsRedirectWithoutAuth(t *testing.T) {
	body := "2026-10-07T04:28:23.9358429Z line one\n2026-10-07T04:28:23.9363430Z line two\n"
	blob := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("blob request carried Authorization %q", got)
		}
		if r.URL.Query().Get("sig") != "s" {
			t.Errorf("signed query lost: %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer blob.Close()
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/actions/jobs/7/logs" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("API request missing auth")
		}
		http.Redirect(w, r, blob.URL+"/log?sig=s", http.StatusFound)
	})
	got, err := c.JobLog(context.Background(), "o", "r", 7)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("body = %q", got)
	}
}

func TestJobLogNotReady(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})
	_, err := c.JobLog(context.Background(), "o", "r", 7)
	if !errors.Is(err, ErrLogNotReady) {
		t.Fatalf("err = %v, want ErrLogNotReady", err)
	}
}

func TestJobLogOtherErrors(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	_, err := c.JobLog(context.Background(), "o", "r", 7)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 500 {
		t.Fatalf("err = %v, want 500 APIError", err)
	}
}

// GitHub redirects in-progress jobs to a blob that does not exist until the
// job completes; that is "not ready", not a failure.
func TestJobLogBlobNotFoundIsNotReady(t *testing.T) {
	blob := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><Error><Code>BlobNotFound</Code></Error>`))
	}))
	defer blob.Close()
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, blob.URL+"/job-logs.txt", http.StatusFound)
	})
	_, err := c.JobLog(context.Background(), "o", "r", 7)
	if !errors.Is(err, ErrLogNotReady) {
		t.Fatalf("err = %v, want ErrLogNotReady", err)
	}
}

func TestJobLogDownloadNotCutByClientTimeout(t *testing.T) {
	blob := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		for i := 0; i < 3; i++ {
			time.Sleep(60 * time.Millisecond)
			_, _ = w.Write([]byte("line\n"))
			w.(http.Flusher).Flush()
		}
	}))
	defer blob.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, blob.URL+"/log", http.StatusFound)
	}))
	defer api.Close()
	c := New("tok", WithBaseURL(api.URL), WithHTTPClient(&http.Client{Timeout: 100 * time.Millisecond}))
	got, err := c.JobLog(context.Background(), "o", "r", 7)
	if err != nil || string(got) != "line\nline\nline\n" {
		t.Fatalf("got %q err %v", got, err)
	}
}
