package poller

import (
	"testing"
	"time"

	"ghtui/internal/gh"
)

const fullLog = "2026-10-07T04:28:23.1Z ##[group]Run make\n2026-10-07T04:28:24.2Z ok\n"

// tailing returns a harness with run 1 focused and job 10 open in Tail.
func tailing(t *testing.T, jobStatus string) *harness {
	t.Helper()
	h := started(t, "in_progress")
	h.api.jobs[1] = []gh.Job{{ID: 10, RunID: 1, Name: "build", Status: jobStatus}}
	h.st.SetFocusRun(1)
	h.st.SetTailJob("a", "x", 10)
	return h
}

func TestLogNotFetchedWhileJobRuns(t *testing.T) {
	h := tailing(t, "in_progress")
	h.api.logs[10] = []logResp{{body: fullLog}}
	for s := 1; s <= 20; s++ {
		h.tickAt(time.Duration(s) * time.Second)
	}
	if n := countCalls(h.api.takeCalls(), "JobLog"); n != 0 {
		t.Fatalf("JobLog called %d times for a running job", n)
	}
}

func TestLogFetchedOnceJobCompletes(t *testing.T) {
	h := tailing(t, "completed")
	h.api.logs[10] = []logResp{{body: fullLog}}
	h.tickAt(time.Second)
	msgs := h.takeMsgs()
	if !hasMsg(msgs, LogAppended{JobID: 10, From: 0, To: 2}) || !hasMsg(msgs, LogComplete{JobID: 10}) {
		t.Fatalf("msgs = %#v", msgs)
	}
	b := h.st.Log(10)
	if len(b.Lines) != 2 || !b.Complete || b.Lines[1].Text != "ok" {
		t.Fatalf("buffer = %+v", b)
	}
	for s := 2; s <= 30; s++ {
		h.tickAt(time.Duration(s) * time.Second)
	}
	if n := countCalls(h.api.takeCalls(), "JobLog"); n != 1 {
		t.Fatalf("JobLog calls = %d, want 1", n)
	}
}

func TestLogNotReadyAfterCompletionRetriesThenErrors(t *testing.T) {
	h := tailing(t, "completed")
	for s := 1; s <= 40; s++ {
		h.tickAt(time.Duration(s) * time.Second)
	}
	if n := countCalls(h.api.takeCalls(), "JobLog"); n != 4 {
		t.Fatalf("JobLog calls = %d, want 1 + 3 retries", n)
	}
	var errs int
	for _, m := range h.takeMsgs() {
		if pe, ok := m.(PollerError); ok && pe.Resource == "log:10" {
			errs++
		}
	}
	if errs != 1 {
		t.Fatalf("PollerError for log sent %d times", errs)
	}
}

func TestTailJobUnknownToStoreUsesGetJob(t *testing.T) {
	h := started(t, "in_progress")
	h.st.SetTailJob("a", "x", 77)
	h.api.job[77] = gh.Job{ID: 77, Status: "completed"}
	h.api.logs[77] = []logResp{{body: fullLog}}
	h.tickAt(time.Second)
	calls := h.api.takeCalls()
	if countCalls(calls, "GetJob 77") != 1 || countCalls(calls, "JobLog 77") != 1 {
		t.Fatalf("calls = %v", calls)
	}
}

func TestChangingTailJobStopsOldOne(t *testing.T) {
	h := tailing(t, "in_progress")
	h.tickAt(time.Second)
	h.st.SetTailJob("", "", 0)
	h.api.jobs[1] = []gh.Job{{ID: 10, RunID: 1, Status: "completed"}}
	h.api.logs[10] = []logResp{{body: fullLog}}
	h.api.takeCalls()
	for s := 2; s <= 20; s++ {
		h.tickAt(time.Duration(s) * time.Second)
	}
	if n := countCalls(h.api.takeCalls(), "JobLog"); n != 0 {
		t.Fatalf("JobLog called %d times after tail closed", n)
	}
}

func TestCompleteBufferNotRefetched(t *testing.T) {
	h := tailing(t, "completed")
	h.api.logs[10] = []logResp{{body: fullLog}}
	h.tickAt(time.Second)
	h.st.SetTailJob("", "", 0)
	h.tickAt(2 * time.Second)
	h.st.SetTailJob("a", "x", 10)
	h.tickAt(3 * time.Second)
	if n := countCalls(h.api.takeCalls(), "JobLog"); n != 1 {
		t.Fatalf("JobLog calls = %d, want 1 (buffer already complete)", n)
	}
}
