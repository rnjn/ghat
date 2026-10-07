package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghtui/internal/gh"
	"github.com/rnjn/ghtui/internal/poller"
)

// run executes cmd and every command batched inside it, returning all msgs.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			// skip the blocking channel reader and the ticker
			if c == nil {
				continue
			}
			out = append(out, safeRun(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func safeRun(c tea.Cmd) []tea.Msg {
	if c == nil {
		return nil
	}
	done := make(chan []tea.Msg, 1)
	go func() { done <- runCmd(c) }()
	select {
	case m := <-done:
		return m
	case <-time.After(50 * time.Millisecond):
		return nil
	}
}

func sendPoller(m Model, msg any) (Model, tea.Cmd) { return update(m, pollerMsg{msg: msg}) }

func TestRunCompletedRingsBellAndFlashes(t *testing.T) {
	m, _ := newTestModel(t)
	run := gh.Run{ID: 12, RepoKey: "acme/api", Number: 41, WorkflowName: "CI", Status: "completed", Conclusion: "failure"}
	m, cmd := sendPoller(m, poller.RunCompleted{Run: run})
	bell := false
	for _, msg := range runCmd(cmd) {
		if r, ok := msg.(tea.RawMsg); ok && r.Msg == "\a" {
			bell = true
		}
	}
	if !bell {
		t.Fatal("no bell")
	}
	if v := plain(m.View().Content); !strings.Contains(v, "acme/api #41 CI: failure") {
		t.Fatalf("no flash:\n%s", v)
	}
	m.ctx.Now = func() time.Time { return testNow.Add(6 * time.Second) }
	if v := plain(m.View().Content); strings.Contains(v, "acme/api #41 CI: failure") {
		t.Fatal("flash still shown after 5s")
	}
}

func TestTickKeepsRefreshing(t *testing.T) {
	m, _ := newTestModel(t)
	_, cmd := update(m, tickMsg{})
	if cmd == nil {
		t.Fatal("tick not re-armed")
	}
}

func TestForceRefreshPerScreen(t *testing.T) {
	ctx, rec := testContext(seedStore())
	seedJobs(ctx.Store)
	m := NewModel(*ctx, make(chan any))
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 12})
	m.ctx.Refresh = func(r string) { rec.refreshed = append(rec.refreshed, r) }
	m, _ = update(m, key("R"))
	m, _ = update(m, Push{Screen: NewRuns("acme/api")})
	m, _ = update(m, key("R"))
	m, _ = update(m, Push{Screen: NewJobs(run12(ctx.Store))})
	m, _ = update(m, key("R"))
	m, _ = update(m, Push{Screen: NewTail("acme", "api", gh.Job{ID: 120, RunID: 12}, 0)})
	_, _ = update(m, key("R"))
	want := "repos runs:acme/api jobs:12 log:120"
	if got := strings.Join(rec.refreshed, " "); got != want {
		t.Fatalf("refreshed %q, want %q", got, want)
	}
}

func TestAuthFailedModal(t *testing.T) {
	m, _ := newTestModel(t)
	m, _ = sendPoller(m, poller.AuthFailed{Err: errors.New("401")})
	v := plain(m.View().Content)
	if !strings.Contains(v, "GitHub rejected the token") || !strings.Contains(v, "gh auth login") {
		t.Fatalf("no modal:\n%s", v)
	}
	m, _ = update(m, key("enter"))
	m, _ = update(m, key("?"))
	if strings.Contains(plain(m.View().Content), "Keys") || m.top().Title() != "Board" {
		t.Fatal("keys other than q handled under the auth modal")
	}
	_, cmd := update(m, key("q"))
	if cmd == nil {
		t.Fatal("q ignored under modal")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q did not quit")
	}
}
