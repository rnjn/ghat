package gh

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRateLimitErrorNamesResetTime(t *testing.T) {
	reset := time.Date(2026, 10, 8, 14, 30, 0, 0, time.Local)
	got := (&RateLimitError{Reset: reset}).Error()
	if !strings.Contains(got, "rate limit exceeded") || !strings.Contains(got, reset.Format(time.Kitchen)) {
		t.Fatalf("message %q", got)
	}
}

func TestAPIErrorMessages(t *testing.T) {
	for _, tc := range []struct {
		err  *APIError
		want []string
	}{
		{&APIError{Status: 502}, []string{"502", "Bad Gateway"}},
		{&APIError{Status: 404, Message: "Not Found"}, []string{"404", "Not Found"}},
		{&APIError{Status: 401, Message: "Bad credentials"}, []string{"401", "Bad credentials", "GH_TOKEN", "gh auth login"}},
	} {
		got := tc.err.Error()
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%+v: message %q lacks %q", tc.err, got, w)
			}
		}
	}
}

func TestIsNotFoundSeesThroughWrapping(t *testing.T) {
	wrapped := fmt.Errorf("listing: %w", &APIError{Status: 404})
	if !IsNotFound(wrapped) || IsNotFound(errors.New("404")) || IsNotFound(&APIError{Status: 500}) {
		t.Fatal("IsNotFound wrong")
	}
}

func TestPermissionErrorMessage(t *testing.T) {
	got := (&PermissionError{Status: 403, Message: "Resource not accessible"}).Error()
	if !strings.HasPrefix(got, "no permission (403)") || !strings.Contains(got, "Resource not accessible") {
		t.Fatalf("message %q", got)
	}
}
