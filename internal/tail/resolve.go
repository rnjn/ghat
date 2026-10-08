package tail

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rnjn/ghat/internal/gh"
)

// ErrAmbiguousRun means a run ID was given but no single job can be chosen.
var ErrAmbiguousRun = errors.New("run has several candidate jobs; pass a job ID")

// AmbiguousRunError lists the jobs of a run that could not be narrowed down.
type AmbiguousRunError struct {
	RunID int64
	Jobs  []gh.Job
}

func (e *AmbiguousRunError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "run %d has %d jobs; pass one of these job IDs:", e.RunID, len(e.Jobs))
	for _, j := range e.Jobs {
		fmt.Fprintf(&b, "\n  %d  %s  (%s)", j.ID, j.Name, j.Status)
	}
	return b.String()
}

func (e *AmbiguousRunError) Is(target error) bool { return target == ErrAmbiguousRun }

// ResolveJob treats id as a job ID, and failing that as a run ID: a run
// resolves to its only job, or its only in-progress job.
func ResolveJob(ctx context.Context, c *gh.Client, owner, repo string, id int64) (gh.Job, error) {
	j, err := c.GetJob(ctx, owner, repo, id)
	if err == nil || !gh.IsNotFound(err) {
		return j, err
	}
	jobs, err := c.ListJobs(ctx, owner, repo, id)
	if gh.IsNotFound(err) {
		return gh.Job{}, fmt.Errorf("no job or run %d in %s/%s: %w", id, owner, repo, err)
	}
	if err != nil {
		return gh.Job{}, err
	}
	switch len(jobs) {
	case 0:
		return gh.Job{}, fmt.Errorf("run %d has no jobs yet", id)
	case 1:
		return jobs[0], nil
	}
	var active []gh.Job
	for _, j := range jobs {
		if j.Status == "in_progress" {
			active = append(active, j)
		}
	}
	if len(active) == 1 {
		return active[0], nil
	}
	return gh.Job{}, &AmbiguousRunError{RunID: id, Jobs: jobs}
}
