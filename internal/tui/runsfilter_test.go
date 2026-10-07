package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghtui/internal/gh"
	"github.com/rnjn/ghtui/internal/poller"
)

func filterRuns(s Screen, ctx *Context, text string) Screen {
	s, _ = press(s, ctx, "/")
	return typeText(s, ctx, text)
}

func TestRunsFilterLive(t *testing.T) {
	ctx, _ := testContext(seedStore())
	s := filterRuns(NewRuns("acme/api"), ctx, "MAIN")
	v := plain(s.View(ctx, 100, 8))
	if strings.Contains(v, "feat/login") || !strings.Contains(v, "#40") || !strings.Contains(v, "#39") {
		t.Fatalf("branch filter:\n%s", v)
	}
	if !strings.Contains(s.Title(), "/main") {
		t.Fatalf("title %q", s.Title())
	}
	s, _ = press(s, ctx, "enter") // keep filter, keys back to the list
	s, _ = press(s, ctx, "j")
	_, cmd := s.Update(key("enter"), ctx)
	if got := pushed(t, cmd).Title(); !strings.Contains(got, "#39") {
		t.Fatalf("enter opened %q", got)
	}
}

func TestRunsFilterByStatus(t *testing.T) {
	ctx, _ := testContext(seedStore())
	s := filterRuns(NewRuns("acme/api"), ctx, "fail")
	v := plain(s.View(ctx, 100, 8))
	if !strings.Contains(v, "#39") || strings.Contains(v, "#40") || strings.Contains(v, "#41") {
		t.Fatalf("status filter:\n%s", v)
	}
}

func TestRunsFilterNoMatch(t *testing.T) {
	ctx, _, fa := actCtx(t)
	s := filterRuns(NewRuns("acme/api"), ctx, "zzz")
	s, _ = press(s, ctx, "enter")
	if v := plain(s.View(ctx, 100, 8)); !strings.Contains(v, "no runs match zzz") {
		t.Fatalf("view:\n%s", v)
	}
	for _, k := range []string{"enter", "r", "x", "w"} {
		if _, cmd := s.Update(key(k), ctx); cmd != nil {
			t.Fatalf("%s produced a command on an empty filter", k)
		}
	}
	runs := ctx.Store.Runs("acme/api")
	runs = append([]gh.Run{{ID: 13, RepoKey: "acme/api", Number: 42, Status: "queued", CreatedAt: ago(time.Second)}}, runs...)
	ctx.Store.SetRuns("acme/api", runs, "")
	s, _ = s.Update(poller.RunsUpdated{RepoKey: "acme/api"}, ctx)
	_ = s.View(ctx, 100, 8)
	if len(fa.calls) != 0 || ctx.Store.Watched(13) {
		t.Fatalf("calls %v", fa.calls)
	}
}

func TestRunsFilterEsc(t *testing.T) {
	ctx, _ := testContext(seedStore())
	m := NewModel(*ctx, make(chan any))
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 10})
	m, _ = update(m, Push{Screen: NewRuns("acme/api")})
	m, _ = update(m, key("/"))
	for _, k := range []string{"m", "a", "i", "n"} {
		m, _ = update(m, key(k))
	}
	m, _ = update(m, key("enter"))
	if strings.Contains(plain(m.View().Content), "feat/login") {
		t.Fatal("filter not applied")
	}
	m, _ = update(m, key("esc")) // clears the filter, stays on Runs
	if m.top().Title() != "acme/api" || !strings.Contains(plain(m.View().Content), "feat/login") {
		t.Fatalf("esc did not clear the filter: top %q", m.top().Title())
	}
	m, _ = update(m, key("esc"))
	if m.top().Title() != "Board" {
		t.Fatal("esc without a filter did not go back")
	}
}
