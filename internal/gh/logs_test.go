package gh

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
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
