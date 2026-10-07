package poller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// syncInterest schedules jobs polls for the focused and watched runs and a
// log poll for the tail job, and drops polls nobody is looking at.
func (p *Poller) syncInterest() {
	want := map[string]bool{}
	if id := p.st.FocusRun(); id != 0 {
		want[jobsKey(id)] = true
	}
	for _, id := range p.st.WatchedRuns() {
		want[jobsKey(id)] = true
	}
	if _, _, id := p.st.TailJob(); id != 0 {
		want[logKey(id)] = true
		if id != p.tailJob {
			p.tailJob, p.logRetries, p.logDone = id, 0, false
		}
	} else {
		p.tailJob = 0
	}
	for _, prefix := range []string{"jobs:", "log:"} {
		p.sched.removePrefix(prefix, func(k string) bool { return want[k] })
	}
	for k := range p.jobsDone {
		if !want[k] {
			delete(p.jobsDone, k)
		}
	}
	for k := range want {
		if !p.jobsDone[k] && !(strings.HasPrefix(k, "log:") && p.logDone) {
			p.sched.setIfAbsent(k, time.Time{})
		}
	}
}

func jobsKey(runID int64) string { return fmt.Sprintf("jobs:%d", runID) }
func logKey(jobID int64) string  { return fmt.Sprintf("log:%d", jobID) }

func keyID(key string) int64 {
	_, s, _ := strings.Cut(key, ":")
	id, _ := strconv.ParseInt(s, 10, 64)
	return id
}

// pollJobs fetches a run's jobs every jobs interval while the run is
// active, and once more after it completes.
func (p *Poller) pollJobs(ctx context.Context, now time.Time, key string) {
	runID := keyID(key)
	run, ok := p.st.Run(runID)
	if !ok {
		p.sched.remove(key)
		return
	}
	owner, repo, _ := strings.Cut(run.RepoKey, "/")
	jobs, err := p.api.ListJobs(ctx, owner, repo, runID)
	if err != nil {
		p.send(PollerError{Resource: key, Err: err})
		p.sched.set(key, now.Add(p.cfg.Poll.Jobs.D()))
		return
	}
	p.st.SetJobs(runID, jobs)
	p.send(JobsUpdated{RunID: runID})
	if run.Status == "completed" {
		p.jobsDone[key] = true
		p.sched.remove(key)
		return
	}
	p.sched.set(key, now.Add(p.cfg.Poll.Jobs.D()))
}
