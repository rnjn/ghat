package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	"ghtui/internal/gh"
	"ghtui/internal/poller"
	"ghtui/internal/tail"
)

func tailCtx(t *testing.T, jobID int64, step int) (*Context, Screen) {
	t.Helper()
	ctx, _ := testContext(seedStore())
	seedJobs(ctx.Store)
	ctx.Store.SetFocusRun(12)
	var job gh.Job
	for _, j := range ctx.Store.Jobs(12) {
		if j.ID == jobID {
			job = j
		}
	}
	ctx.Store.SetTailJob("acme", "api", jobID)
	return ctx, NewTail("acme", "api", job, step)
}

// logLines builds n plain lines; every 10th starts a step group.
func logLines(n int) []tail.LogLine {
	out := make([]tail.LogLine, n)
	for i := range out {
		out[i] = tail.LogLine{Text: fmt.Sprintf("line %d", i), StepNumber: 1 + i/10,
			Timestamp: time.Date(2026, 10, 7, 4, 28, 0, 0, time.UTC).Add(time.Duration(i) * time.Second)}
	}
	return out
}

func TestTailRunningShowsStepsAndSpinner(t *testing.T) {
	ctx, s := tailCtx(t, 120, 0)
	golden.RequireEqual(t, plain(s.View(ctx, 80, 8)))
}

func TestTailCompletedShowsStyledLog(t *testing.T) {
	ctx, s := tailCtx(t, 121, 0)
	ctx.Store.SetLog(121, []tail.LogLine{
		{Text: "Run golangci-lint", Kind: tail.Group, StepNumber: 2},
		{Text: "main.go:3: unused", Kind: tail.Plain, StepNumber: 2},
		{Kind: tail.EndGroup, StepNumber: 2},
		{Text: "lint failed", Kind: tail.Error, StepNumber: 2},
		{Text: "deprecated", Kind: tail.Warning, StepNumber: 2},
	}, true)
	s, _ = s.Update(poller.LogAppended{JobID: 121, From: 0, To: 5}, ctx)
	v := s.View(ctx, 80, 6)
	golden.RequireEqual(t, plain(v))
	if !strings.Contains(v, styleError.Render("error: lint failed")) || !strings.Contains(v, styleWarning.Render("warning: deprecated")) {
		t.Fatalf("errors/warnings not styled:\n%q", v)
	}
}

func TestTailFollowPauseAndResume(t *testing.T) {
	ctx, s := tailCtx(t, 121, 0)
	ctx.Store.SetLog(121, logLines(100), false)
	v := plain(s.View(ctx, 60, 6))
	if !strings.Contains(v, "line 99") {
		t.Fatalf("follow mode not at end:\n%s", v)
	}
	s, _ = s.Update(key("k"), ctx)
	ctx.Store.SetLog(121, logLines(120), true)
	s, _ = s.Update(poller.LogAppended{JobID: 121, From: 100, To: 120}, ctx)
	if v := plain(s.View(ctx, 60, 6)); strings.Contains(v, "line 119") || !strings.Contains(v, "line 98") {
		t.Fatalf("scrolling up did not pause follow:\n%s", v)
	}
	s, _ = s.Update(key("G"), ctx)
	if v := plain(s.View(ctx, 60, 6)); !strings.Contains(v, "line 119") {
		t.Fatalf("G did not resume follow:\n%s", v)
	}
}

func TestTailTimestampsToggle(t *testing.T) {
	ctx, s := tailCtx(t, 121, 0)
	ctx.Store.SetLog(121, logLines(3), true)
	ts := logLines(1)[0].Timestamp.Local().Format("15:04:05")
	if strings.Contains(plain(s.View(ctx, 80, 5)), ts) {
		t.Fatal("timestamps shown by default")
	}
	s, _ = s.Update(key("t"), ctx)
	if !strings.Contains(plain(s.View(ctx, 80, 5)), ts) {
		t.Fatal("t did not show timestamps")
	}
}

func TestTailOpensAtTargetStep(t *testing.T) {
	ctx, s := tailCtx(t, 121, 5)
	ctx.Store.SetLog(121, logLines(100), true)
	v := plain(s.View(ctx, 60, 6))
	if !strings.HasPrefix(strings.TrimSpace(v), "line 40") {
		t.Fatalf("not scrolled to step 5 (line 40):\n%s", v)
	}
}

func TestTailLogErrorShown(t *testing.T) {
	ctx, s := tailCtx(t, 121, 0)
	s, _ = s.Update(poller.PollerError{Resource: "log:121", Err: errors.New("gone")}, ctx)
	if !strings.Contains(plain(s.View(ctx, 80, 5)), "log not available yet / expired") {
		t.Fatal("log error not shown")
	}
	_, _ = s.Update(poller.PollerError{Resource: "log:999", Err: errors.New("other")}, ctx)
}

func TestTailWaitingForLog(t *testing.T) {
	ctx, s := tailCtx(t, 121, 0)
	if !strings.Contains(plain(s.View(ctx, 80, 5)), "fetching log") {
		t.Fatal("no fetching message for a completed job without a log")
	}
}

func TestTailPopClearsTailJob(t *testing.T) {
	ctx, s := tailCtx(t, 121, 0)
	s.(popper).OnPop(ctx)
	if _, _, id := ctx.Store.TailJob(); id != 0 {
		t.Fatal("tail job not cleared")
	}
}

func TestTailLargeLogRendersOnlyWindow(t *testing.T) {
	ctx, s := tailCtx(t, 121, 0)
	ctx.Store.SetLog(121, logLines(50000), true)
	_ = s.View(ctx, 120, 40) // first render builds the index
	start := time.Now()
	for i := 0; i < 10; i++ {
		_ = s.View(ctx, 120, 40)
	}
	if d := time.Since(start) / 10; d > 50*time.Millisecond {
		t.Fatalf("View took %v per frame", d)
	}
	if n := strings.Count(s.View(ctx, 120, 40), "\n"); n != 39 {
		t.Fatalf("view has %d lines", n+1)
	}
}

func TestTailResizeKeepsScrollValid(t *testing.T) {
	ctx, s := tailCtx(t, 121, 0)
	ctx.Store.SetLog(121, logLines(30), true)
	_ = s.View(ctx, 40, 10)
	s, _ = s.Update(key("g"), ctx)
	for i := 0; i < 50; i++ {
		s, _ = s.Update(key("j"), ctx)
	}
	v := plain(s.View(ctx, 40, 10))
	if !strings.Contains(v, "line 29") || strings.Contains(v, "line 20\n") {
		t.Fatalf("j did not stop at the end:\n%s", v)
	}
	big := plain(s.View(ctx, 120, 40))
	if !strings.Contains(big, "line 0") || !strings.Contains(big, "line 29") {
		t.Fatalf("grown window does not show the whole log:\n%s", big)
	}
	assertFits(t, s.View(ctx, 40, 10), 40)
	_ = s.View(ctx, 0, 0)
}
