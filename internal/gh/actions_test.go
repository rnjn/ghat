package gh

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type captured struct {
	method, path, auth string
	body               map[string]any
}

func writeServer(t *testing.T, status int, respBody string, got *captured) *Client {
	t.Helper()
	return newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path, got.auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &got.body)
		}
		w.Header().Set("X-RateLimit-Remaining", "4321")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	})
}

func TestWriteCallsHitTheRightEndpoints(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		call func(*Client) error
		path string
	}{
		"rerun":        {func(c *Client) error { return c.RerunRun(ctx, "o", "r", 9) }, "/repos/o/r/actions/runs/9/rerun"},
		"rerun-failed": {func(c *Client) error { return c.RerunFailedJobs(ctx, "o", "r", 9) }, "/repos/o/r/actions/runs/9/rerun-failed-jobs"},
		"cancel":       {func(c *Client) error { return c.CancelRun(ctx, "o", "r", 9) }, "/repos/o/r/actions/runs/9/cancel"},
	} {
		for _, status := range []int{201, 202, 204} {
			var got captured
			c := writeServer(t, status, "", &got)
			if err := tc.call(c); err != nil {
				t.Fatalf("%s %d: %v", name, status, err)
			}
			if got.method != http.MethodPost || got.path != tc.path || got.auth != "Bearer tok" {
				t.Fatalf("%s: got %+v", name, got)
			}
			if rem, _ := c.RateLimit(); rem != 4321 {
				t.Fatalf("%s: rate limit not recorded", name)
			}
		}
	}
}

func TestDispatchWorkflowBody(t *testing.T) {
	var got captured
	c := writeServer(t, 204, "", &got)
	err := c.DispatchWorkflow(context.Background(), "o", "r", 77, "main", map[string]string{"env": "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if got.path != "/repos/o/r/actions/workflows/77/dispatches" || got.body["ref"] != "main" {
		t.Fatalf("got %+v", got)
	}
	if in, _ := got.body["inputs"].(map[string]any); in["env"] != "prod" {
		t.Fatalf("inputs = %#v", got.body["inputs"])
	}
	got = captured{}
	c = writeServer(t, 204, "", &got)
	_ = c.DispatchWorkflow(context.Background(), "o", "r", 77, "main", nil)
	if _, ok := got.body["inputs"]; ok {
		t.Fatalf("empty inputs sent: %#v", got.body)
	}
}

func TestWritePermissionErrors(t *testing.T) {
	for _, status := range []int{403, 404} {
		var got captured
		c := writeServer(t, status, `{"message":"Resource not accessible by integration"}`, &got)
		err := c.CancelRun(context.Background(), "o", "r", 9)
		var pe *PermissionError
		if !errors.As(err, &pe) || pe.Status != status {
			t.Fatalf("%d: err = %T %v", status, err, err)
		}
		msg := err.Error()
		if !strings.Contains(msg, "no permission") || !strings.Contains(msg, "write access to this repository") ||
			strings.Contains(msg, "workflow") || !strings.Contains(msg, "Resource not accessible") {
			t.Fatalf("%d: message %q", status, msg)
		}
	}
}

func TestWriteConflictKeepsGitHubMessage(t *testing.T) {
	var got captured
	c := writeServer(t, 409, `{"message":"Cannot cancel a workflow run that is completed."}`, &got)
	err := c.CancelRun(context.Background(), "o", "r", 9)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 409 || !strings.Contains(ae.Message, "completed") {
		t.Fatalf("err = %v", err)
	}
}
