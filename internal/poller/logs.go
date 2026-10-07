package poller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ghtui/internal/gh"
	"ghtui/internal/tail"
)

// logRetries is how many times a completed job's missing log is refetched.
const maxLogRetries = 3

// pollLog fetches the tail job's log once the job has completed: GitHub
// only publishes the log at completion.
func (p *Poller) pollLog(ctx context.Context, now time.Time, key string) {
	owner, repo, jobID := p.st.TailJob()
	if jobID != keyID(key) {
		p.sched.remove(key)
		return
	}
	if p.st.Log(jobID).Complete {
		p.finishLog(key)
		return
	}
	job, err := p.tailJobState(ctx, owner, repo, jobID)
	if err != nil {
		p.send(PollerError{Resource: key, Err: err})
		p.sched.set(key, now.Add(p.cfg.Poll.Jobs.D()))
		return
	}
	if job.Status != "completed" {
		p.sched.set(key, now.Add(p.cfg.Poll.Jobs.D()))
		return
	}
	raw, err := p.api.JobLog(ctx, owner, repo, jobID)
	if err != nil {
		if errors.Is(err, gh.ErrLogNotReady) && p.logRetries < maxLogRetries {
			p.logRetries++
			p.sched.set(key, now.Add(p.cfg.Poll.Jobs.D()))
			return
		}
		if errors.Is(err, gh.ErrLogNotReady) {
			err = fmt.Errorf("log not available yet or expired: %w", err)
			p.finishLog(key)
		} else {
			p.sched.set(key, now.Add(p.cfg.Poll.Jobs.D()))
		}
		p.send(PollerError{Resource: key, Err: err})
		return
	}
	from, to := p.st.SetLog(jobID, tail.ParseLines(raw, job.Steps), true)
	if to > from {
		p.send(LogAppended{JobID: jobID, From: from, To: to})
	}
	p.send(LogComplete{JobID: jobID})
	p.finishLog(key)
}

func (p *Poller) finishLog(key string) {
	p.logDone = true
	p.sched.remove(key)
}

// tailJobState finds the job in the focused run's jobs, else asks GitHub.
func (p *Poller) tailJobState(ctx context.Context, owner, repo string, jobID int64) (gh.Job, error) {
	if run := p.st.FocusRun(); run != 0 {
		for _, j := range p.st.Jobs(run) {
			if j.ID == jobID {
				return j, nil
			}
		}
	}
	return p.api.GetJob(ctx, owner, repo, jobID)
}
