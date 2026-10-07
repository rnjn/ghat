package poller

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"ghtui/internal/config"
	"ghtui/internal/gh"
)

func TestFirstTickDiscoversThenPollsEveryRepo(t *testing.T) {
	h := newHarness(ghRepo("a", "x", time.Hour), ghRepo("a", "y", 2*time.Hour))
	h.tickAt(0)
	calls := strings.Join(h.api.takeCalls(), "|")
	if calls != "ListRepos|ListRuns a/x etag=|ListRuns a/y etag=" {
		t.Fatalf("calls = %s", calls)
	}
	msgs := withoutRate(h.takeMsgs())
	if len(msgs) != 3 {
		t.Fatalf("msgs = %#v", msgs)
	}
	if _, ok := msgs[0].(ReposUpdated); !ok {
		t.Fatalf("first msg = %#v", msgs[0])
	}
	if m, ok := msgs[1].(RunsUpdated); !ok || m.RepoKey != "a/x" {
		t.Fatalf("second msg = %#v", msgs[1])
	}
}

func TestDiscoveryAppliesWindowPinnedAndExclude(t *testing.T) {
	h := newHarness(ghRepo("a", "x", time.Hour), ghRepo("a", "skip", time.Hour))
	cfg := config.Default()
	cfg.Repos.Pinned = []string{"a/x", "b/pinned"}
	cfg.Repos.Exclude = []string{"a/skip"}
	h.p = New(h.api, h.st, cfg, func(m any) { h.msgs = append(h.msgs, m) })
	h.api.getRepo["b/pinned"] = ghRepo("b", "pinned", 90*24*time.Hour)
	h.tickAt(0)
	if want := h.now.Add(-14 * 24 * time.Hour); !h.api.since.Equal(want) {
		t.Fatalf("since = %v, want %v", h.api.since, want)
	}
	var got []string
	for _, r := range h.st.Repos() {
		got = append(got, fmt.Sprintf("%s:%v", r.Repo.Key(), r.Pinned))
	}
	if fmt.Sprint(got) != "[a/x:true b/pinned:true]" {
		t.Fatalf("repos = %v", got)
	}
	if calls := h.api.takeCalls(); countCalls(calls, "GetRepo") != 1 {
		t.Fatalf("GetRepo should be called only for undiscovered pinned repos: %v", calls)
	}
}

func TestPinnedRepoLookupFailureMarksUnavailable(t *testing.T) {
	h := newHarness()
	cfg := config.Default()
	cfg.Repos.Pinned = []string{"b/gone"}
	h.p = New(h.api, h.st, cfg, func(m any) { h.msgs = append(h.msgs, m) })
	h.tickAt(0)
	rs := h.st.Repos()
	if len(rs) != 1 || !rs[0].Unavailable || !rs[0].Pinned {
		t.Fatalf("repos = %+v", rs)
	}
}

func TestDiscoveryRepeatsEveryTenMinutes(t *testing.T) {
	h := newHarness(ghRepo("a", "x", time.Hour))
	h.tickAt(0)
	h.api.takeCalls()
	h.tickAt(10*time.Minute - time.Second)
	if n := countCalls(h.api.takeCalls(), "ListRepos"); n != 0 {
		t.Fatalf("rediscovered early (%d)", n)
	}
	h.tickAt(10 * time.Minute)
	if n := countCalls(h.api.takeCalls(), "ListRepos"); n != 1 {
		t.Fatalf("ListRepos calls = %d", n)
	}
}

func TestRunsIntervalActiveVersusIdle(t *testing.T) {
	for name, tc := range map[string]struct {
		status string
		next   time.Duration
	}{"active": {"in_progress", 15 * time.Second}, "idle": {"completed", 60 * time.Second}} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(ghRepo("a", "x", time.Hour))
			h.api.runs["a/x"] = []gh.Run{ghRun(1, "a/x", tc.status, "")}
			h.tickAt(0)
			h.api.takeCalls()
			h.tickAt(tc.next - time.Second)
			if n := countCalls(h.api.takeCalls(), "ListRuns"); n != 0 {
				t.Fatalf("polled early")
			}
			h.tickAt(tc.next)
			if n := countCalls(h.api.takeCalls(), "ListRuns"); n != 1 {
				t.Fatalf("ListRuns calls = %d", n)
			}
		})
	}
}

func TestRunPollsStaggered(t *testing.T) {
	h := newHarness(ghRepo("a", "r0", time.Minute), ghRepo("a", "r1", 2*time.Minute), ghRepo("a", "r2", 3*time.Minute), ghRepo("a", "r3", 4*time.Minute))
	h.tickAt(0)
	h.api.takeCalls()
	for i, at := range []time.Duration{15, 30, 45, 60} {
		h.tickAt(at * time.Second)
		calls := h.api.takeCalls()
		want := fmt.Sprintf("ListRuns a/r%d etag=", i)
		if len(calls) != 1 || calls[0] != want {
			t.Fatalf("at %ds calls = %v, want [%s]", at, calls, want)
		}
	}
}

func TestRunsETagNotModifiedSendsNothing(t *testing.T) {
	h := newHarness(ghRepo("a", "x", time.Hour))
	h.api.runsETag["a/x"] = `"e1"`
	h.tickAt(0)
	h.takeMsgs()
	h.api.takeCalls()
	h.tickAt(time.Minute)
	if calls := h.api.takeCalls(); len(calls) != 1 || calls[0] != `ListRuns a/x etag="e1"` {
		t.Fatalf("calls = %v", calls)
	}
	if msgs := h.takeMsgs(); len(msgs) != 0 {
		t.Fatalf("msgs on 304 = %#v", msgs)
	}
}

func TestWatchedRunCompletionSendsRunCompletedOnce(t *testing.T) {
	h := newHarness(ghRepo("a", "x", time.Hour))
	h.api.runs["a/x"] = []gh.Run{ghRun(1, "a/x", "in_progress", "")}
	h.tickAt(0)
	h.st.ToggleWatch(1)
	h.takeMsgs()
	h.api.runs["a/x"] = []gh.Run{ghRun(1, "a/x", "completed", "success")}
	h.tickAt(15 * time.Second)
	h.tickAt(30 * time.Second)
	n := 0
	for _, m := range h.takeMsgs() {
		if rc, ok := m.(RunCompleted); ok {
			n++
			if rc.Run.ID != 1 || rc.Run.Conclusion != "success" {
				t.Fatalf("RunCompleted = %+v", rc)
			}
		}
	}
	if n != 1 {
		t.Fatalf("RunCompleted sent %d times", n)
	}
}

func TestRefreshMakesResourceDueNow(t *testing.T) {
	h := newHarness(ghRepo("a", "x", time.Hour))
	h.tickAt(0)
	h.api.takeCalls()
	h.p.Refresh("runs:a/x")
	h.tickAt(time.Second)
	if n := countCalls(h.api.takeCalls(), "ListRuns"); n != 1 {
		t.Fatalf("ListRuns calls after refresh = %d", n)
	}
	h.p.Refresh("repos")
	h.tickAt(2 * time.Second)
	if n := countCalls(h.api.takeCalls(), "ListRepos"); n != 1 {
		t.Fatalf("ListRepos calls after refresh = %d", n)
	}
}

func withoutRate(msgs []any) []any {
	var out []any
	for _, m := range msgs {
		if _, ok := m.(RateLimit); !ok {
			out = append(out, m)
		}
	}
	return out
}
