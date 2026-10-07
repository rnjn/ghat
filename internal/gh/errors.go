package gh

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// RateLimitError is returned when GitHub rejects a request because the
// primary rate limit is exhausted.
type RateLimitError struct {
	Reset time.Time
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("GitHub API rate limit exceeded, resets at %s", e.Reset.Local().Format(time.Kitchen))
}

// APIError is any other non-2xx response from GitHub.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Status == http.StatusUnauthorized {
		return fmt.Sprintf("GitHub API: 401 %s: check GH_TOKEN/GITHUB_TOKEN or run `gh auth login`", e.Message)
	}
	if e.Message == "" {
		return fmt.Sprintf("GitHub API: %d %s", e.Status, http.StatusText(e.Status))
	}
	return fmt.Sprintf("GitHub API: %d %s", e.Status, e.Message)
}

// IsNotFound reports whether err is a 404 APIError.
func IsNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

// IsTransient reports whether err is worth retrying: a 5xx response or a
// network failure. Client errors, rate limits, a missing log and context
// cancellation are not.
func IsTransient(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrLogNotReady) {
		return false
	}
	var rl *RateLimitError
	if errors.As(err, &rl) {
		return false
	}
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status >= 500
	}
	return true
}
