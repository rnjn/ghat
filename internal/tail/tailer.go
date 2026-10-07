package tail

import (
	"context"
	"errors"
	"time"

	"ghtui/internal/gh"
)

// Source is the subset of the GitHub client the tailer needs.
type Source interface {
	JobLog(ctx context.Context, owner, repo string, jobID int64) ([]byte, error)
	GetJob(ctx context.Context, owner, repo string, id int64) (gh.Job, error)
}

// Options tunes a Tailer. Zero values get defaults.
type Options struct {
	Interval time.Duration                                    // poll interval, default 5s
	Sleep    func(ctx context.Context, d time.Duration) error // injectable for tests
}

// Tailer follows one job's log, emitting each line once as steps complete.
type Tailer struct {
	src     Source
	owner   string
	repo    string
	jobID   int64
	opts    Options
	emitted int
}

// NewTailer returns a tailer for jobID in owner/repo.
func NewTailer(src Source, owner, repo string, jobID int64, opts Options) *Tailer {
	if opts.Interval <= 0 {
		opts.Interval = 5 * time.Second
	}
	if opts.Sleep == nil {
		opts.Sleep = SleepContext
	}
	return &Tailer{src: src, owner: owner, repo: repo, jobID: jobID, opts: opts}
}

// Run polls until the job completes, calling emit for every new line, and
// returns the completed job.
func (t *Tailer) Run(ctx context.Context, emit func(LogLine)) (gh.Job, error) {
	for {
		job, err := t.src.GetJob(ctx, t.owner, t.repo, t.jobID)
		if err != nil {
			return gh.Job{}, err
		}
		lines, err := t.poll(ctx, job, emit)
		if err != nil {
			return job, err
		}
		if job.Status == "completed" {
			return job, t.finish(ctx, job, lines, emit)
		}
		if err := t.opts.Sleep(ctx, t.opts.Interval); err != nil {
			return job, err
		}
	}
}

// finalRetries is how many extra fetches a completed job gets while its log
// lags the status flip.
const finalRetries = 3

// finish refetches a completed job's log until it holds the final step's
// output, at most finalRetries times. Running out of retries is not an error.
func (t *Tailer) finish(ctx context.Context, job gh.Job, lines []LogLine, emit func(LogLine)) error {
	for i := 0; i < finalRetries && !logComplete(lines, job.Steps); i++ {
		if err := t.opts.Sleep(ctx, t.opts.Interval); err != nil {
			return err
		}
		var err error
		if lines, err = t.poll(ctx, job, emit); err != nil {
			return err
		}
	}
	return nil
}

// logComplete reports whether a completed job's log includes output from
// its last step: some line stamped at or after that step's start. Without
// step times, any non-empty log counts as complete.
func logComplete(lines []LogLine, steps []gh.Step) bool {
	if len(lines) == 0 {
		return false
	}
	var last time.Time
	for _, s := range steps {
		if !s.StartedAt.IsZero() {
			last = s.StartedAt
		}
	}
	if last.IsZero() {
		return true
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if !lines[i].Timestamp.IsZero() && !lines[i].Timestamp.Before(last) {
			return true
		}
	}
	return false
}

// poll fetches the log once and emits lines past the emitted count. It
// returns the full parsed log. Completed step output is immutable, so a
// line-count diff is safe; the whole log is parsed each time so step
// attribution sees every group header.
func (t *Tailer) poll(ctx context.Context, job gh.Job, emit func(LogLine)) ([]LogLine, error) {
	raw, err := t.src.JobLog(ctx, t.owner, t.repo, t.jobID)
	if errors.Is(err, gh.ErrLogNotReady) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lines := ParseLines(raw, job.Steps)
	for _, l := range lines[min(t.emitted, len(lines)):] {
		emit(l)
	}
	t.emitted = max(t.emitted, len(lines))
	return lines, nil
}

// SleepContext sleeps for d or until ctx is done, returning ctx.Err() then.
func SleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
