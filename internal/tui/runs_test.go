package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	"ghtui/internal/gh"
	"ghtui/internal/poller"
)

func TestRunsGolden(t *testing.T) {
	ctx, _ := testContext(seedStore())
	ctx.Store.ToggleWatch(12)
	golden.RequireEqual(t, plain(NewRuns("acme/api").View(ctx, 100, 6)))
}

func TestRunsKeepsSelectionByIDOnUpdate(t *testing.T) {
	ctx, _ := testContext(seedStore())
	s := NewRuns("acme/api")
	s, _ = s.Update(key("j"), ctx) // run 11
	runs := ctx.Store.Runs("acme/api")
	runs = append([]gh.Run{{ID: 13, RepoKey: "acme/api", Number: 42, Status: "queued", CreatedAt: ago(time.Second)}}, runs...)
	ctx.Store.SetRuns("acme/api", runs, "")
	s, _ = s.Update(poller.RunsUpdated{RepoKey: "acme/api"}, ctx)
	_, cmd := s.Update(key("enter"), ctx)
	if got := pushed(t, cmd).Title(); !strings.Contains(got, "#40") {
		t.Fatalf("selection moved: pushed %q, want run #40", got)
	}
}

func TestRunsCursorClampsWhenRunVanishes(t *testing.T) {
	ctx, _ := testContext(seedStore())
	s := NewRuns("acme/api")
	s, _ = s.Update(key("G"), ctx) // run 10, last
	ctx.Store.SetRuns("acme/api", ctx.Store.Runs("acme/api")[:1], "")
	s, _ = s.Update(poller.RunsUpdated{RepoKey: "acme/api"}, ctx)
	_ = s.View(ctx, 80, 5)
	_, cmd := s.Update(key("enter"), ctx)
	if got := pushed(t, cmd).Title(); !strings.Contains(got, "#41") {
		t.Fatalf("pushed %q, want the only remaining run #41", got)
	}
}

func TestRunsEnterFocusesAndRefreshes(t *testing.T) {
	ctx, rec := testContext(seedStore())
	_, cmd := NewRuns("acme/api").Update(key("enter"), ctx)
	pushed(t, cmd)
	if ctx.Store.FocusRun() != 12 {
		t.Fatalf("focus = %d", ctx.Store.FocusRun())
	}
	if len(rec.refreshed) != 1 || rec.refreshed[0] != "jobs:12" {
		t.Fatalf("refreshed = %v", rec.refreshed)
	}
}

func TestRunsPopClearsFocus(t *testing.T) {
	ctx, _ := testContext(seedStore())
	ctx.Store.SetFocusRun(12)
	NewRuns("acme/api").(popper).OnPop(ctx)
	if ctx.Store.FocusRun() != 0 {
		t.Fatal("focus not cleared")
	}
}

func TestRunsWatchToggle(t *testing.T) {
	ctx, _ := testContext(seedStore())
	s := NewRuns("acme/api")
	s, _ = s.Update(key("w"), ctx)
	if !ctx.Store.Watched(12) {
		t.Fatal("w did not watch")
	}
	_, _ = s.Update(key("w"), ctx)
	if ctx.Store.Watched(12) {
		t.Fatal("second w did not unwatch")
	}
}

func TestRunsEmptyAndTiny(t *testing.T) {
	ctx, _ := testContext(seedStore())
	s := NewRuns("acme/nothing")
	if !strings.Contains(plain(s.View(ctx, 60, 4)), "no runs") {
		t.Fatal("empty message missing")
	}
	if _, cmd := s.Update(key("enter"), ctx); cmd != nil {
		t.Fatal("enter on empty list pushed")
	}
	assertFits(t, NewRuns("acme/api").View(ctx, 40, 10), 40)
	_ = NewRuns("acme/api").View(ctx, 0, 0)
}
