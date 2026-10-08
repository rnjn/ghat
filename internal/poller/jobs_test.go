package poller

import (
	"strings"
	"testing"
	"time"

	"github.com/rnjn/ghat/internal/gh"
)

// started returns a harness with one repo whose run 1 has the given status,
// after the first tick.
func started(t *testing.T, runStatus string) *harness {
	t.Helper()
	h := newHarness(ghRepo("a", "x", time.Hour))
	h.api.runs["a/x"] = []gh.Run{ghRun(1, "a/x", runStatus, ""), ghRun(2, "a/x", "in_progress", "")}
	h.api.jobs[1] = []gh.Job{{ID: 10, RunID: 1, Name: "build", Status: "in_progress"}}
	h.tickAt(0)
	h.api.takeCalls()
	h.takeMsgs()
	return h
}

func TestJobsNotPolledForUninterestingRuns(t *testing.T) {
	h := started(t, "in_progress")
	for s := 1; s <= 30; s++ {
		h.tickAt(time.Duration(s) * time.Second)
	}
	if n := countCalls(h.api.takeCalls(), "ListJobs"); n != 0 {
		t.Fatalf("ListJobs called %d times with nothing focused or watched", n)
	}
}

func TestFocusedRunPolledEveryFiveSeconds(t *testing.T) {
	h := started(t, "in_progress")
	h.st.SetFocusRun(1)
	h.tickAt(time.Second)
	if calls := h.api.takeCalls(); countCalls(calls, "ListJobs 1") != 1 {
		t.Fatalf("calls = %v", calls)
	}
	if !hasMsg(h.takeMsgs(), JobsUpdated{RunID: 1}) {
		t.Fatal("no JobsUpdated")
	}
	if len(h.st.Jobs(1)) != 1 {
		t.Fatal("jobs not stored")
	}
	h.tickAt(5 * time.Second)
	if n := countCalls(h.api.takeCalls(), "ListJobs"); n != 0 {
		t.Fatal("polled before 5s elapsed")
	}
	h.tickAt(6 * time.Second)
	if n := countCalls(h.api.takeCalls(), "ListJobs 1"); n != 1 {
		t.Fatalf("ListJobs calls at +5s = %d", n)
	}
}

func TestWatchedRunPolled(t *testing.T) {
	h := started(t, "in_progress")
	h.st.ToggleWatch(2)
	h.tickAt(time.Second)
	if n := countCalls(h.api.takeCalls(), "ListJobs 2"); n != 1 {
		t.Fatal("watched run's jobs not polled")
	}
}

func TestCompletedRunJobsFetchedOnceThenStop(t *testing.T) {
	h := started(t, "completed")
	h.st.SetFocusRun(1)
	h.tickAt(time.Second)
	h.tickAt(10 * time.Second)
	h.tickAt(20 * time.Second)
	if n := countCalls(h.api.takeCalls(), "ListJobs 1"); n != 1 {
		t.Fatalf("ListJobs calls = %d, want 1", n)
	}
}

func TestUnfocusingStopsJobPolling(t *testing.T) {
	h := started(t, "in_progress")
	h.st.SetFocusRun(1)
	h.tickAt(time.Second)
	h.st.SetFocusRun(0)
	h.tickAt(10 * time.Second)
	if n := countCalls(h.api.takeCalls(), "ListJobs"); n != 1 {
		t.Fatalf("ListJobs calls = %d, want 1", n)
	}
}

func hasMsg(msgs []any, want any) bool {
	for _, m := range msgs {
		if m == want {
			return true
		}
	}
	return false
}

func TestRerunOfCompletedRunPollsJobsAgain(t *testing.T) {
	h := started(t, "completed")
	h.st.SetFocusRun(1)
	h.tickAt(time.Second)
	h.api.takeCalls()
	h.api.runs["a/x"] = []gh.Run{ghRun(1, "a/x", "in_progress", ""), ghRun(2, "a/x", "in_progress", "")}
	h.tickAt(60 * time.Second) // runs poll sees the rerun
	h.tickAt(61 * time.Second)
	if n := countCalls(h.api.takeCalls(), "ListJobs 1"); n != 1 {
		t.Fatalf("ListJobs after rerun = %d, want 1", n)
	}
}

func TestFocusedRunDroppedByDiscoveryStopsPolling(t *testing.T) {
	h := started(t, "in_progress")
	h.st.SetFocusRun(1)
	h.tickAt(time.Second)
	h.api.repos = nil // repo falls out of the push window
	h.p.Refresh("repos")
	h.tickAt(2 * time.Second)
	h.api.takeCalls()
	for s := 3; s < 30; s++ {
		h.tickAt(time.Duration(s) * time.Second)
	}
	if n := countCalls(h.api.takeCalls(), "ListJobs"); n != 0 {
		t.Fatalf("ListJobs called %d times for an untracked run", n)
	}
	if _, ok := h.p.sched.due["jobs:1"]; ok {
		t.Fatal("jobs key still scheduled")
	}
}

func TestFocusDuringLongTickIsServedBeforeRemainingRunsPolls(t *testing.T) {
	h := started(t, "completed")
	h.api.repos = append(h.api.repos, ghRepo("a", "y", time.Hour), ghRepo("a", "z", time.Hour))
	h.p.Refresh("repos")
	focused := false
	h.api.onRuns = func() {
		if !focused { // the user opens run 1 while the first runs poll is in flight
			focused = true
			h.st.SetFocusRun(1)
		}
	}
	h.tickAt(time.Second)
	calls := h.api.takeCalls()
	jobsAt := -1
	runsAfter := 0
	for i, c := range calls {
		if c == "ListJobs 1" {
			jobsAt = i
		} else if jobsAt >= 0 && strings.HasPrefix(c, "ListRuns") {
			runsAfter++
		}
	}
	if jobsAt < 0 || runsAfter == 0 {
		t.Fatalf("jobs not served ahead of the queued runs polls: %v", calls)
	}
}
