// Package poller owns every GitHub call the TUI makes. It writes results
// into the store and sends change messages for the UI.
package poller

import (
	"time"

	"github.com/rnjn/ghtui/internal/gh"
)

// ReposUpdated means the discovered repo set changed.
type ReposUpdated struct{}

// Polled means a poll succeeded, changed or not (304); it keeps the
// status bar's "last poll" age honest.
type Polled struct{}

// RunsFailed means a repo's runs poll hit a network error or 5xx; the
// poller is retrying and the UI keeps the last good state.
type RunsFailed struct {
	RepoKey string
	Err     error
}

// RunsUpdated means a repo's runs changed.
type RunsUpdated struct{ RepoKey string }

// JobsUpdated means a run's jobs changed.
type JobsUpdated struct{ RunID int64 }

// LogAppended means lines [From, To) were added to a job's log buffer.
type LogAppended struct {
	JobID    int64
	From, To int
}

// LogComplete means a job's log is final.
type LogComplete struct{ JobID int64 }

// RunCompleted means a watched run finished.
type RunCompleted struct{ Run gh.Run }

// RateLimit reports the API quota.
type RateLimit struct {
	Remaining int
	Reset     time.Time
}

// PollerError reports a failed poll of one resource.
type PollerError struct {
	Resource string
	Err      error
}

// AuthFailed means GitHub rejected the token; polling has stopped.
type AuthFailed struct{ Err error }
