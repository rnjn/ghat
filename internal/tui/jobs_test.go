package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/rnjn/ghtui/internal/gh"
	"github.com/rnjn/ghtui/internal/poller"
)

func jobsCtx(t *testing.T) (*Context, Screen) {
	t.Helper()
	ctx, _ := testContext(seedStore())
	seedJobs(ctx.Store)
	return ctx, NewJobs(run12(ctx.Store))
}

func TestJobsGolden(t *testing.T) {
	ctx, s := jobsCtx(t)
	golden.RequireEqual(t, plain(s.View(ctx, 100, 7)))
}

func TestJobsNarrowShowsJobsOnly(t *testing.T) {
	ctx, s := jobsCtx(t)
	v := plain(s.View(ctx, 50, 7))
	if !strings.Contains(v, "build") || strings.Contains(v, "Checkout") {
		t.Fatalf("narrow view:\n%s", v)
	}
	assertFits(t, s.View(ctx, 50, 7), 50)
	assertFits(t, s.View(ctx, 40, 10), 40)
	_ = s.View(ctx, 0, 0)
}

func TestJobsLoading(t *testing.T) {
	ctx, _ := testContext(seedStore())
	if !strings.Contains(plain(NewJobs(run12(ctx.Store)).View(ctx, 80, 5)), "loading jobs") {
		t.Fatal("no loading message")
	}
}

func TestJobsSelectingJobChangesSteps(t *testing.T) {
	ctx, s := jobsCtx(t)
	s, _ = s.Update(key("j"), ctx)
	v := plain(s.View(ctx, 100, 7))
	if !strings.Contains(v, "Run golangci-lint") || strings.Contains(v, "Checkout") {
		t.Fatalf("steps pane not following selection:\n%s", v)
	}
}

func TestJobsEnterOnJobOpensTailAtTop(t *testing.T) {
	ctx, s := jobsCtx(t)
	_, cmd := s.Update(key("enter"), ctx)
	tl := pushed(t, cmd).(*tailScreen)
	if tl.job.ID != 120 || tl.step != 0 {
		t.Fatalf("tail job %d step %d", tl.job.ID, tl.step)
	}
	if o, r, id := ctx.Store.TailJob(); o != "acme" || r != "api" || id != 120 {
		t.Fatalf("tail job = %s/%s %d", o, r, id)
	}
}

func TestJobsEnterOnStepOpensTailAtStep(t *testing.T) {
	ctx, s := jobsCtx(t)
	for _, k := range []string{"tab", "j", "j"} {
		s, _ = s.Update(key(k), ctx)
	}
	_, cmd := s.Update(key("enter"), ctx)
	if tl := pushed(t, cmd).(*tailScreen); tl.job.ID != 120 || tl.step != 3 {
		t.Fatalf("tail job %d step %d, want 120 step 3", tl.job.ID, tl.step)
	}
	s, _ = s.Update(key("left"), ctx)
	s, _ = s.Update(key("j"), ctx)
	_, cmd = s.Update(key("enter"), ctx)
	if tl := pushed(t, cmd).(*tailScreen); tl.job.ID != 121 || tl.step != 0 {
		t.Fatalf("after left+j: tail job %d step %d", tl.job.ID, tl.step)
	}
}

func TestJobsKeepSelectionByIDAndClamp(t *testing.T) {
	ctx, s := jobsCtx(t)
	s, _ = s.Update(key("j"), ctx) // lint (121)
	jobs := ctx.Store.Jobs(12)
	ctx.Store.SetJobs(12, []gh.Job{{ID: 119, Name: "setup", Status: "completed"}, jobs[0], jobs[1]})
	s, _ = s.Update(poller.JobsUpdated{RunID: 12}, ctx)
	_, cmd := s.Update(key("enter"), ctx)
	if tl := pushed(t, cmd).(*tailScreen); tl.job.ID != 121 {
		t.Fatalf("selection moved to %d", tl.job.ID)
	}
	ctx.Store.SetJobs(12, jobs[:1])
	s, _ = s.Update(poller.JobsUpdated{RunID: 12}, ctx)
	_, cmd = s.Update(key("enter"), ctx)
	if tl := pushed(t, cmd).(*tailScreen); tl.job.ID != 120 {
		t.Fatalf("after shrink selected %d, want 120", tl.job.ID)
	}
}

func TestJobsPopClearsFocus(t *testing.T) {
	ctx, s := jobsCtx(t)
	ctx.Store.SetFocusRun(12)
	s.(popper).OnPop(ctx)
	if ctx.Store.FocusRun() != 0 {
		t.Fatal("focus not cleared")
	}
}

func TestJobsTailInheritsShowTimestamps(t *testing.T) {
	ctx, s := jobsCtx(t)
	ctx.ShowTimestamps = true
	_, cmd := s.Update(key("enter"), ctx)
	if tl := pushed(t, cmd).(*tailScreen); !tl.timestamps {
		t.Fatal("ui.show_timestamps not applied to Tail")
	}
}

func TestJobsRunNoLongerTracked(t *testing.T) {
	ctx, s := jobsCtx(t)
	ctx.Store.SetRepos(nil)
	if v := plain(s.View(ctx, 80, 5)); !strings.Contains(v, "run no longer tracked") {
		t.Fatalf("view:\n%s", v)
	}
}
