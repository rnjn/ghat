package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghtui/internal/tail"
)

func searchLog() []tail.LogLine {
	return []tail.LogLine{
		{Text: "Run setup", Kind: tail.Group},        // 0
		{Text: "fetch [x] done"},                     // 1
		{Kind: tail.EndGroup},                        // 2
		{Text: "compile a.b"},                        // 3
		{Text: "FAILING test one", Kind: tail.Error}, // 4
		{Text: "axb not a regex hit"},                // 5
		{Text: "failing test two"},                   // 6
	}
}

func searchCtx(t *testing.T) (*Context, Screen) {
	t.Helper()
	ctx, s := tailCtx(t, 121, 0)
	ctx.Store.SetLog(121, searchLog(), true)
	_ = s.View(ctx, 80, 12)
	return ctx, s
}

func search(s Screen, ctx *Context, q string) Screen {
	s, _ = press(s, ctx, "/")
	s = typeText(s, ctx, q)
	s, _ = press(s, ctx, "enter")
	return s
}

func TestTailSearchJumpsAndCycles(t *testing.T) {
	ctx, s := searchCtx(t)
	s, _ = press(s, ctx, "g")
	s = search(s, ctx, "failing")
	if got := cursorText(t, s, ctx); got != "FAILING test one" {
		t.Fatalf("first hit %q", got)
	}
	if v := plain(s.View(ctx, 80, 12)); !strings.Contains(v, "hit 1 of 2") {
		t.Fatalf("footer:\n%s", v)
	}
	s, _ = press(s, ctx, "n")
	if got := cursorText(t, s, ctx); got != "failing test two" {
		t.Fatalf("n → %q", got)
	}
	s, _ = press(s, ctx, "n") // wraps
	if got := cursorText(t, s, ctx); got != "FAILING test one" {
		t.Fatalf("wrap → %q", got)
	}
	s, _ = press(s, ctx, "N") // wraps backwards
	if got := cursorText(t, s, ctx); got != "failing test two" {
		t.Fatalf("N → %q", got)
	}
}

func TestTailSearchIsLiteral(t *testing.T) {
	for q, want := range map[string]string{"[x]": "fetch [x] done", "a.b": "compile a.b", "(": ""} {
		ctx, s := searchCtx(t)
		s, _ = press(s, ctx, "g")
		s = search(s, ctx, q)
		v := plain(s.View(ctx, 80, 12))
		if want == "" {
			if !strings.Contains(v, "no matches for (") {
				t.Fatalf("%q: footer:\n%s", q, v)
			}
			continue
		}
		if got := cursorText(t, s, ctx); got != want || !strings.Contains(v, "hit 1 of 1") {
			t.Fatalf("%q: cursor %q, footer:\n%s", q, got, v)
		}
	}
}

func TestTailSearchUnfoldsHit(t *testing.T) {
	ctx, s := searchCtx(t)
	s, _ = press(s, ctx, "g", "z") // fold "Run setup"
	s = search(s, ctx, "[x]")
	if got := cursorText(t, s, ctx); got != "fetch [x] done" {
		t.Fatalf("hit inside folded group not shown: %q", got)
	}
}

func TestTailSearchHighlightsMatches(t *testing.T) {
	ctx, s := searchCtx(t)
	s = search(s, ctx, "test")
	if v := s.View(ctx, 80, 12); !strings.Contains(v, styleMatch.Render("test")) {
		t.Fatalf("no highlight:\n%q", v)
	}
}

func TestTailSearchEscClosesOnlyTheSearch(t *testing.T) {
	ctx, _ := testContext(seedStore())
	seedJobs(ctx.Store)
	ctx.Store.SetLog(121, searchLog(), true)
	m := NewModel(*ctx, make(chan any))
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 12})
	m, _ = update(m, Push{Screen: NewTail("acme", "api", ctx.Store.Jobs(12)[1], 0)})
	m, _ = update(m, key("/"))
	for _, k := range []string{"q", "n", "r"} { // typed, not commands
		var cmd tea.Cmd
		m, cmd = update(m, key(k))
		for _, msg := range safeRun(cmd) {
			if _, quit := msg.(tea.QuitMsg); quit {
				t.Fatalf("%s quit while searching", k)
			}
		}
	}
	if !strings.Contains(plain(m.View().Content), "/qnr") {
		t.Fatalf("query not shown:\n%s", plain(m.View().Content))
	}
	m, _ = update(m, key("esc"))
	if m.top().Title() != "lint" {
		t.Fatalf("esc popped the screen; top = %q", m.top().Title())
	}
	m, _ = update(m, key("esc"))
	if m.top().Title() == "lint" {
		t.Fatal("second esc did not pop")
	}
}

func TestTailSearchFastOn50kLines(t *testing.T) {
	ctx, s := tailCtx(t, 121, 0)
	lines := make([]tail.LogLine, 50000)
	for i := range lines {
		lines[i] = tail.LogLine{Text: fmt.Sprintf("line %d of the log with some text", i)}
	}
	lines[49000].Text = "needle here"
	ctx.Store.SetLog(121, lines, true)
	_ = s.View(ctx, 120, 40)
	s, _ = press(s, ctx, "g", "/")
	s = typeText(s, ctx, "needle")
	start := time.Now()
	s, _ = press(s, ctx, "enter")
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("search took %v", d)
	}
	if got := cursorText(t, s, ctx); got != "needle here" {
		t.Fatalf("cursor %q", got)
	}
}
