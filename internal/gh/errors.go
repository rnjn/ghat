package gh

import (
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
