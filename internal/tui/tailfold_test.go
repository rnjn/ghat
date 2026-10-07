package tui

import (
	"strings"
	"testing"

	"ghtui/internal/tail"
)

// groupedLog: header A with 2 lines, a plain line, header B with 3 lines.
func groupedLog() []tail.LogLine {
	return []tail.LogLine{
		{Text: "Run A", Kind: tail.Group}, // 0
		{Text: "a1"}, {Text: "a2"},        // 1, 2
		{Kind: tail.EndGroup},                    // 3
		{Text: "between"},                        // 4
		{Text: "Run B", Kind: tail.Group},        // 5
		{Text: "b1"}, {Text: "b2"}, {Text: "b3"}, // 6-8
		{Kind: tail.EndGroup}, // 9
	}
}

func foldCtx(t *testing.T) (*Context, Screen) {
	t.Helper()
	ctx, s := tailCtx(t, 121, 0)
	ctx.Store.SetLog(121, groupedLog(), true)
	_ = s.View(ctx, 60, 12)
	return ctx, s
}

func cursorText(t *testing.T, s Screen, ctx *Context) string {
	t.Helper()
	ts := s.(*tailScreen)
	lines := ctx.Store.Log(121).Lines
	return lines[ts.ix.vis[ts.cur]].Text
}

func TestTailCursorMoves(t *testing.T) {
	ctx, s := foldCtx(t)
	if got := cursorText(t, s, ctx); got != "b3" {
		t.Fatalf("follow cursor on %q, want last line b3", got)
	}
	s, _ = press(s, ctx, "k", "k")
	if got := cursorText(t, s, ctx); got != "b1" {
		t.Fatalf("after k k cursor on %q", got)
	}
	s, _ = press(s, ctx, "g")
	if got := cursorText(t, s, ctx); got != "Run A" {
		t.Fatalf("after g cursor on %q", got)
	}
	s, _ = press(s, ctx, "G")
	_ = s.View(ctx, 60, 12)
	if got := cursorText(t, s, ctx); got != "b3" {
		t.Fatalf("after G cursor on %q", got)
	}
}

func TestTailFoldAndUnfold(t *testing.T) {
	ctx, s := foldCtx(t)
	s, _ = press(s, ctx, "g", "j") // a1, inside group A
	s, _ = press(s, ctx, "z")
	v := plain(s.View(ctx, 60, 12))
	if strings.Contains(v, "a1") || !strings.Contains(v, "▸ Run A (2 lines)") || !strings.Contains(v, "b1") {
		t.Fatalf("group A not folded:\n%s", v)
	}
	if got := cursorText(t, s, ctx); got != "Run A" {
		t.Fatalf("cursor on %q after fold, want the header", got)
	}
	s, _ = press(s, ctx, "j") // skips folded lines
	if got := cursorText(t, s, ctx); got != "between" {
		t.Fatalf("j landed on %q", got)
	}
	s, _ = press(s, ctx, "k", "z") // header: unfold
	if v := plain(s.View(ctx, 60, 12)); !strings.Contains(v, "a1") || !strings.Contains(v, "▾ Run A") {
		t.Fatalf("not unfolded:\n%s", v)
	}
	s, _ = press(s, ctx, "z")
	s, _ = press(s, ctx, "G")
	_ = s.View(ctx, 60, 12)
	s, _ = press(s, ctx, "z") // fold B from its last line
	s, _ = press(s, ctx, "Z")
	if v := plain(s.View(ctx, 60, 12)); !strings.Contains(v, "a1") || !strings.Contains(v, "b3") {
		t.Fatalf("Z did not expand all:\n%s", v)
	}
}

func TestTailFoldedGroupKeepsGrowingHidden(t *testing.T) {
	ctx, s := tailCtx(t, 121, 0)
	ctx.Store.SetLog(121, groupedLog()[:8], false) // B open, b1 b2 so far
	_ = s.View(ctx, 60, 12)
	s, _ = press(s, ctx, "z") // cursor on b2 (follow) → fold B
	ctx.Store.SetLog(121, groupedLog(), true)
	v := plain(s.View(ctx, 60, 12))
	if strings.Contains(v, "b3") || !strings.Contains(v, "▸ Run B (3 lines)") {
		t.Fatalf("appended line not folded or count stale:\n%s", v)
	}
	if got := cursorText(t, s, ctx); got != "Run B" {
		t.Fatalf("cursor on %q, want visible header", got)
	}
	assertFits(t, s.View(ctx, 40, 10), 40)
}

func TestTailSurvivesShrinkingLog(t *testing.T) {
	ctx, s := foldCtx(t)
	s, _ = press(s, ctx, "k")
	s = search(s, ctx, "b")
	ctx.Store.DropLog(121)
	ctx.Store.SetLog(121, groupedLog()[:2], true)
	_ = s.View(ctx, 60, 12)
	for _, k := range []string{"z", "Z", "n", "N", "j", "k", "G"} {
		s, _ = press(s, ctx, k)
		_ = s.View(ctx, 60, 12)
	}
	s = search(s, ctx, "a1")
	if got := cursorText(t, s, ctx); got != "a1" {
		t.Fatalf("cursor %q", got)
	}
}
