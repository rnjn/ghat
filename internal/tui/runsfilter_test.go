package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/poller"
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

// seedMoreRuns adds a timed-out and a skipped run to acme/api.
func seedMoreRuns(ctx *Context) {
	runs := ctx.Store.Runs("acme/api")
	runs = append(runs,
		gh.Run{ID: 9, RepoKey: "acme/api", Number: 38, WorkflowName: "CI", Branch: "main", Status: "completed", Conclusion: "timed_out", CreatedAt: ago(4 * time.Hour), UpdatedAt: ago(4 * time.Hour)},
		gh.Run{ID: 8, RepoKey: "acme/api", Number: 37, WorkflowName: "CI", Branch: "main", Status: "completed", Conclusion: "skipped", CreatedAt: ago(5 * time.Hour), UpdatedAt: ago(5 * time.Hour)},
	)
	ctx.Store.SetRuns("acme/api", runs, "")
}

func visibleNumbers(s Screen, ctx *Context) string {
	var out []string
	for _, run := range s.(*runsScreen).visible(ctx) {
		out = append(out, fmt.Sprintf("#%d", run.Number))
	}
	return strings.Join(out, " ")
}

func TestRunsFilterTermsAndNegation(t *testing.T) {
	ctx, _ := testContext(seedStore())
	seedMoreRuns(ctx)
	for text, want := range map[string]string{
		"main failure":      "#39",
		"main -fail":        "#40 #38 #37",
		"-skipped":          "#41 #40 #39 #38",
		"-skipped -timed":   "#41 #40 #39",
		"  main   -skipped ": "#40 #39 #38",
		"-":                 "#41 #40 #39 #38 #37",
	} {
		if got := visibleNumbers(filterRuns(NewRuns("acme/api"), ctx, text), ctx); got != want {
			t.Errorf("/%s: %s, want %s", text, got, want)
		}
	}
}

func TestRunsFilterAliases(t *testing.T) {
	ctx, _ := testContext(seedStore())
	seedMoreRuns(ctx)
	for text, want := range map[string]string{
		"failed":  "#39 #38",
		"active":  "#41",
		"queued":  "",
		"done":    "#40 #39 #38 #37",
		"-failed": "#41 #40 #37",
		"-done":   "#41",
	} {
		if got := visibleNumbers(filterRuns(NewRuns("acme/api"), ctx, text), ctx); got != want {
			t.Errorf("/%s: %s, want %s", text, got, want)
		}
	}
}

func TestRunsStatusCycle(t *testing.T) {
	ctx, _ := testContext(seedStore())
	seedMoreRuns(ctx)
	s := filterRuns(NewRuns("acme/api"), ctx, "main")
	s, _ = press(s, ctx, "enter")
	for _, want := range []string{"failed", "active", "queued", "done", "", "failed"} {
		s, _ = press(s, ctx, "s")
		if got := s.(*runsScreen).filter; got != want {
			t.Fatalf("after s: filter %q, want %q", got, want)
		}
		if want != "" && !strings.Contains(s.Title(), "/"+want) {
			t.Fatalf("title %q", s.Title())
		}
	}
	if got := visibleNumbers(s, ctx); got != "#39 #38" {
		t.Fatalf("failed preset shows %s", got)
	}
	// Opening / shows the preset so it can be edited.
	s, _ = press(s, ctx, "/")
	if v := s.(*runsScreen).input.Value(); v != "failed" {
		t.Fatalf("input %q", v)
	}
}
