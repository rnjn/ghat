package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghat/internal/poller"
	"github.com/rnjn/ghat/internal/tail"
)

func headerOf(t *testing.T, s Screen, ctx *Context) (string, string) {
	t.Helper()
	h, ok := s.(headed)
	if !ok {
		t.Fatalf("%T has no header", s)
	}
	return h.Heading(ctx), plain(strings.Join(h.Stats(ctx), "\n"))
}

func TestBoardHeader(t *testing.T) {
	ctx, _ := testContext(seedStore())
	ctx.Store.ToggleWatch(12)
	title, stats := headerOf(t, NewBoard(), ctx)
	if title != "Repositories" {
		t.Fatalf("title %q", title)
	}
	for _, want := range []string{"3 repos", "1 unavailable", "1 active (1 running, 0 queued)", "4 runs loaded", "33% success", "2 failed", "1 watched"} {
		if !strings.Contains(stats, want) {
			t.Errorf("stats %q lack %q", stats, want)
		}
	}
}

func TestRunsHeader(t *testing.T) {
	ctx, _ := testContext(seedStore())
	title, stats := headerOf(t, NewRuns("acme/api"), ctx)
	if title != "Runs · acme/api" {
		t.Fatalf("title %q", title)
	}
	for _, want := range []string{"3 runs", "1 active", "50% success of 2 finished", "median 10m0s", "last failure 2h ago"} {
		if !strings.Contains(stats, want) {
			t.Errorf("stats %q lack %q", stats, want)
		}
	}
}

func TestPipelineHeader(t *testing.T) {
	ctx, _ := testContext(seedStore())
	seedJobs(ctx.Store)
	title, stats := headerOf(t, NewJobs(run12(ctx.Store)), ctx)
	if title != "Pipeline · acme/api #41 CI" {
		t.Fatalf("title %q", title)
	}
	for _, want := range []string{"in_progress", "feat/login", "pull_request", "alice", "3m0s", "jobs 1/2 done", "1 failed", "running build › Run make test"} {
		if !strings.Contains(stats, want) {
			t.Errorf("stats %q lack %q", stats, want)
		}
	}
}

func TestLogsHeader(t *testing.T) {
	ctx, s := tailCtx(t, 120, 0)
	ctx.Store.SetLog(120, []tail.LogLine{{Text: "a"}, {Text: "b"}}, false)
	_ = s.View(ctx, 80, 10)
	title, stats := headerOf(t, s, ctx)
	if title != "Logs · build" {
		t.Fatalf("title %q", title)
	}
	for _, want := range []string{"in_progress", "steps 2/4 done", "Run make test 2m40s", "2 lines"} {
		if !strings.Contains(stats, want) {
			t.Errorf("stats %q lack %q", stats, want)
		}
	}
}

func TestModelRendersHeader(t *testing.T) {
	ctx, _ := testContext(seedStore())
	m := NewModel(*ctx, make(chan any))
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 20})
	m, _ = update(m, pollerMsg{msg: poller.RateLimit{Remaining: 4321, Reset: testNow.Add(time.Hour)}})
	lines := strings.Split(plain(m.View().Content), "\n")
	if len(lines) != 20 {
		t.Fatalf("%d lines", len(lines))
	}
	if !strings.HasPrefix(lines[0], " ghat ▸ Repositories") || !strings.Contains(lines[0], "quota 4321") ||
		!strings.Contains(lines[0], testNow.Local().Format("15:04")) {
		t.Fatalf("title line %q", lines[0])
	}
	if !strings.Contains(lines[1], "3 repos") || !strings.HasPrefix(strings.TrimSpace(lines[3]), "────") {
		t.Fatalf("header:\n%s", strings.Join(lines[:4], "\n"))
	}
	if !strings.Contains(lines[4], "REPO") {
		t.Fatalf("body does not start after the header: %q", lines[4])
	}
	if strings.Contains(lines[19], "Repositories") {
		t.Fatalf("status bar still shows the title: %q", lines[19])
	}
	assertFits(t, m.View().Content, 100)
}

func TestModelShortTerminalShowsTitleOnly(t *testing.T) {
	ctx, _ := testContext(seedStore())
	m := NewModel(*ctx, make(chan any))
	m, _ = update(m, tea.WindowSizeMsg{Width: 60, Height: 12})
	lines := strings.Split(plain(m.View().Content), "\n")
	if !strings.Contains(lines[0], "Repositories") || !strings.Contains(lines[1], "REPO") {
		t.Fatalf("view:\n%s", strings.Join(lines[:3], "\n"))
	}
	m, _ = update(m, tea.WindowSizeMsg{Width: 30, Height: 3})
	assertFits(t, m.View().Content, 30)
}
