package tui

import (
	"errors"
	"strings"
	"testing"
)

// screensOnRun12 returns the three screens whose current run is 12 (in
// progress) in the seeded store.
func screensOnRun12(ctx *Context) map[string]Screen {
	jobs := ctx.Store.Jobs(12)
	return map[string]Screen{
		"runs": NewRuns("acme/api"),
		"jobs": NewJobs(run12(ctx.Store)),
		"tail": NewTail("acme", "api", jobs[0], 0),
	}
}

func TestCancelAsksThenCallsOnceAndRefreshes(t *testing.T) {
	for _, name := range []string{"runs", "jobs", "tail"} {
		ctx, rec, fa := actCtx(t)
		s := screensOnRun12(ctx)[name]
		_, cmd := s.Update(key("x"), ctx)
		c := cmd().(Confirm)
		if c.Prompt != "Cancel acme/api #41 CI?" {
			t.Fatalf("%s: prompt %q", name, c.Prompt)
		}
		res := c.Run().(ActionResult)
		if res.Err != nil || res.Text != "cancel requested" || len(fa.calls) != 1 || fa.calls[0] != "cancel 12" {
			t.Fatalf("%s: res %+v calls %v", name, res, fa.calls)
		}
		if strings.Join(rec.refreshed, " ") != "runs:acme/api jobs:12" {
			t.Fatalf("%s: refreshed %v", name, rec.refreshed)
		}
	}
}

func TestRerunOnActiveRunRefusesWithoutAsking(t *testing.T) {
	ctx, _, fa := actCtx(t)
	for name, s := range screensOnRun12(ctx) {
		_, cmd := s.Update(key("r"), ctx)
		res, ok := cmd().(ActionResult)
		if !ok || res.Err == nil || !strings.Contains(res.Err.Error(), "still in progress") {
			t.Fatalf("%s: got %#v", name, res)
		}
	}
	if len(fa.calls) != 0 {
		t.Fatalf("calls %v", fa.calls)
	}
}

func TestCancelOnFinishedRunRefusesWithoutAsking(t *testing.T) {
	ctx, _, fa := actCtx(t)
	s := NewRuns("acme/api")
	s, _ = s.Update(key("j"), ctx) // run 11, completed
	_, cmd := s.Update(key("x"), ctx)
	res, ok := cmd().(ActionResult)
	if !ok || res.Err == nil || !strings.Contains(res.Err.Error(), "already finished") || len(fa.calls) != 0 {
		t.Fatalf("got %#v calls %v", res, fa.calls)
	}
	_, cmd = s.Update(key("r"), ctx)
	if res := confirmed(t, cmd); res.Err != nil || fa.calls[0] != "rerun 11" {
		t.Fatalf("rerun of finished run: %+v %v", res, fa.calls)
	}
}

func TestActionErrorDoesNotRefresh(t *testing.T) {
	ctx, rec, fa := actCtx(t)
	fa.err = errors.New("GitHub refused (403)")
	_, cmd := NewRuns("acme/api").Update(key("x"), ctx)
	if res := confirmed(t, cmd); res.Err == nil {
		t.Fatal("error lost")
	}
	if len(rec.refreshed) != 0 {
		t.Fatalf("refreshed after error: %v", rec.refreshed)
	}
}

func TestActionUsesLatestRunStatus(t *testing.T) {
	ctx, _, fa := actCtx(t)
	s := NewJobs(run12(ctx.Store)) // captured while in progress
	runs := ctx.Store.Runs("acme/api")
	runs[0].Status, runs[0].Conclusion = "completed", "failure"
	ctx.Store.SetRuns("acme/api", runs, "")
	_, cmd := s.Update(key("r"), ctx)
	if res := confirmed(t, cmd); res.Err != nil || fa.calls[0] != "rerun 12" {
		t.Fatalf("res %+v calls %v", res, fa.calls)
	}
}

func TestOpenInBrowser(t *testing.T) {
	ctx, rec, _ := actCtx(t)
	jobs := ctx.Store.Jobs(12)
	noURL := jobs[1] // lint job has no HTMLURL: falls back to the run
	for _, tc := range []struct {
		s    Screen
		want string
	}{
		{NewBoard(), "https://github.com/acme/api/actions"},
		{NewRuns("acme/api"), "https://github.com/acme/api/actions/runs/12"},
		{NewJobs(run12(ctx.Store)), "https://github.com/acme/api/actions/runs/12/job/120"},
		{NewTail("acme", "api", jobs[0], 0), "https://github.com/acme/api/actions/runs/12/job/120"},
		{NewTail("acme", "api", noURL, 0), "https://github.com/acme/api/actions/runs/12"},
	} {
		rec.opened = nil
		_, cmd := tc.s.Update(key("o"), ctx)
		if cmd == nil {
			t.Fatalf("%s: no command", tc.s.Title())
		}
		if res, _ := cmd().(ActionResult); res.Err != nil {
			t.Fatalf("%s: %v", tc.s.Title(), res.Err)
		}
		if len(rec.opened) != 1 || rec.opened[0] != tc.want {
			t.Fatalf("%s: opened %v, want %s", tc.s.Title(), rec.opened, tc.want)
		}
	}
}

func TestOpenErrorAndEmptyList(t *testing.T) {
	ctx, rec, _ := actCtx(t)
	rec.openErr = errors.New("no browser")
	_, cmd := NewRuns("acme/api").Update(key("o"), ctx)
	if res := cmd().(ActionResult); res.Err == nil || !strings.Contains(res.Err.Error(), "no browser") {
		t.Fatalf("res %+v", res)
	}
	rec.opened = nil
	if _, cmd := NewRuns("acme/none").Update(key("o"), ctx); cmd != nil {
		cmd()
	}
	if len(rec.opened) != 0 {
		t.Fatalf("opened %v for an empty list", rec.opened)
	}
}
