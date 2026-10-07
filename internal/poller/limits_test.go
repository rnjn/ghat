package poller

import (
	"testing"
	"time"

	"ghtui/internal/gh"
)

func TestLowRateLimitDoublesIntervals(t *testing.T) {
	h := newHarness(ghRepo("a", "x", time.Hour))
	h.api.remaining = 400
	h.api.reset = h.now.Add(30 * time.Minute)
	h.tickAt(0)
	if !hasMsg(h.takeMsgs(), RateLimit{Remaining: 400, Reset: h.api.reset}) {
		t.Fatal("no RateLimit message")
	}
	h.api.takeCalls()
	h.tickAt(60 * time.Second)
	if n := countCalls(h.api.takeCalls(), "ListRuns"); n != 0 {
		t.Fatal("polled at normal interval despite low quota")
	}
	h.tickAt(120 * time.Second)
	if n := countCalls(h.api.takeCalls(), "ListRuns"); n != 1 {
		t.Fatalf("ListRuns at doubled interval = %d", n)
	}
}

func TestRateLimitMessageOnlyOnChange(t *testing.T) {
	h := newHarness(ghRepo("a", "x", time.Hour))
	h.tickAt(0)
	h.takeMsgs()
	h.tickAt(60 * time.Second)
	for _, m := range h.takeMsgs() {
		if _, ok := m.(RateLimit); ok {
			t.Fatal("RateLimit resent without change")
		}
	}
}

func TestExhaustedQuotaPausesUntilReset(t *testing.T) {
	h := newHarness(ghRepo("a", "x", time.Hour))
	h.api.remaining = 0
	h.api.reset = h.now.Add(5 * time.Minute)
	h.tickAt(0)
	h.tickAt(4 * time.Minute)
	if calls := h.api.takeCalls(); len(calls) != 0 {
		t.Fatalf("calls while paused: %v", calls)
	}
	h.tickAt(5 * time.Minute)
	if n := countCalls(h.api.takeCalls(), "ListRepos"); n != 1 {
		t.Fatal("did not resume after reset")
	}
}

func TestRateLimitErrorPausesUntilReset(t *testing.T) {
	h := newHarness(ghRepo("a", "x", time.Hour))
	h.api.reposErr = &gh.RateLimitError{Reset: h.now.Add(3 * time.Minute)}
	h.tickAt(0)
	h.api.reposErr = nil
	h.api.takeCalls()
	h.p.Refresh("repos")
	h.tickAt(2 * time.Minute)
	if calls := h.api.takeCalls(); len(calls) != 0 {
		t.Fatalf("calls while rate limited: %v", calls)
	}
	h.tickAt(3 * time.Minute)
	if n := countCalls(h.api.takeCalls(), "ListRepos"); n != 1 {
		t.Fatal("did not resume after reset")
	}
}

func TestTransientErrorBacksOffPerResource(t *testing.T) {
	h := started(t, "in_progress")
	h.st.SetFocusRun(1)
	h.api.jobsErr[1] = &gh.APIError{Status: 502}
	var jobTimes []int
	for s := 1; s <= 400; s++ {
		h.tickAt(time.Duration(s) * time.Second)
		if countCalls(h.api.takeCalls(), "ListJobs") > 0 {
			jobTimes = append(jobTimes, s)
		}
	}
	// 5s base: retry after 10, 20, 40, 80, then capped at 120.
	want := []int{1, 11, 31, 71, 151, 271, 391}
	if len(jobTimes) != len(want) {
		t.Fatalf("ListJobs at %v, want %v", jobTimes, want)
	}
	for i := range want {
		if jobTimes[i] != want[i] {
			t.Fatalf("ListJobs at %v, want %v", jobTimes, want)
		}
	}
}

func TestBackoffDoesNotStopOtherResourcesAndResetsOnSuccess(t *testing.T) {
	h := started(t, "in_progress")
	h.st.SetFocusRun(1)
	h.api.jobsErr[1] = &gh.APIError{Status: 503}
	h.tickAt(time.Second)      // fail, next at 11s
	h.tickAt(11 * time.Second) // fail, next at 31s
	h.api.takeCalls()
	h.tickAt(15 * time.Second)
	calls := h.api.takeCalls()
	if countCalls(calls, "ListRuns") != 1 || countCalls(calls, "ListJobs") != 0 {
		t.Fatalf("calls at 15s = %v: runs must keep polling while jobs back off", calls)
	}
	h.api.jobsErr[1] = nil
	h.tickAt(31 * time.Second) // succeeds
	h.api.takeCalls()
	h.tickAt(36 * time.Second)
	if n := countCalls(h.api.takeCalls(), "ListJobs"); n != 1 {
		t.Fatal("interval not reset to 5s after success")
	}
}

func TestRepoUnavailableOn404Or403(t *testing.T) {
	for _, status := range []int{403, 404} {
		h := newHarness(ghRepo("a", "x", time.Hour), ghRepo("a", "y", time.Hour))
		h.api.runsErr["a/x"] = &gh.APIError{Status: status, Message: "Not Found"}
		h.tickAt(0)
		r, _ := h.st.Repo("a/x")
		if !r.Unavailable {
			t.Fatalf("%d: repo not marked unavailable", status)
		}
		h.api.takeCalls()
		for s := 1; s < 600; s += 15 {
			h.tickAt(time.Duration(s) * time.Second)
		}
		calls := h.api.takeCalls()
		if countCalls(calls, "ListRuns a/x") != 0 {
			t.Fatalf("%d: unavailable repo polled again before rediscovery", status)
		}
		if countCalls(calls, "ListRuns a/y") == 0 {
			t.Fatalf("%d: other repo not polled", status)
		}
		h.api.runsErr["a/x"] = nil
		h.tickAt(10 * time.Minute)
		if countCalls(h.api.takeCalls(), "ListRuns a/x") != 1 {
			t.Fatalf("%d: repo not retried after rediscovery", status)
		}
		if r, _ := h.st.Repo("a/x"); r.Unavailable {
			t.Fatalf("%d: still unavailable after rediscovery", status)
		}
	}
}

func TestRepoWithNoRunsIsFine(t *testing.T) {
	h := newHarness(ghRepo("a", "empty", time.Hour))
	h.tickAt(0)
	msgs := h.takeMsgs()
	if !hasMsg(msgs, RunsUpdated{RepoKey: "a/empty"}) {
		t.Fatalf("msgs = %#v", msgs)
	}
	for _, m := range msgs {
		if _, ok := m.(PollerError); ok {
			t.Fatalf("error for empty repo: %#v", m)
		}
	}
}

func TestUnauthorizedStopsPolling(t *testing.T) {
	h := newHarness(ghRepo("a", "x", time.Hour))
	h.api.reposErr = &gh.APIError{Status: 401, Message: "Bad credentials"}
	h.tickAt(0)
	h.tickAt(20 * time.Minute)
	h.p.Refresh("repos")
	h.tickAt(21 * time.Minute)
	if n := countCalls(h.api.takeCalls(), "ListRepos"); n != 1 {
		t.Fatalf("ListRepos calls = %d, want 1", n)
	}
	n := 0
	for _, m := range h.takeMsgs() {
		if _, ok := m.(AuthFailed); ok {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("AuthFailed sent %d times", n)
	}
}
