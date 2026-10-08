package tui

import (
	"strings"
	"testing"
	"time"
)

func TestTableCapsWideColumns(t *testing.T) {
	long := strings.Repeat("x", 100)
	lines := table([]string{"NAME", "DURATION"}, [][]string{{long, "3m0s"}}, 0)
	row := plain(lines[1])
	if !strings.Contains(row, "3m0s") || len([]rune(row)) > maxColWidth+20 {
		t.Fatalf("row %q: wide cell must be cut so later columns stay visible", row)
	}
	if !strings.Contains(row, "…") {
		t.Fatalf("row %q: cut cell has no ellipsis", row)
	}
}

func TestGlyph(t *testing.T) {
	for _, tc := range []struct{ status, conclusion, want string }{
		{"in_progress", "", "●"},
		{"queued", "", "○"}, {"waiting", "", "○"}, {"pending", "", "○"}, {"requested", "", "○"},
		{"completed", "success", "✓"},
		{"completed", "failure", "✗"}, {"completed", "timed_out", "✗"}, {"completed", "startup_failure", "✗"},
		{"completed", "cancelled", "⊘"},
		{"completed", "skipped", "–"}, {"completed", "neutral", "–"}, {"completed", "stale", "–"},
		{"completed", "action_required", "!"},
		{"completed", "something_new", "?"}, {"weird", "", "?"},
	} {
		if got := plain(glyph(tc.status, tc.conclusion)); got != tc.want {
			t.Errorf("glyph(%q, %q) = %q, want %q", tc.status, tc.conclusion, got, tc.want)
		}
	}
}

func TestFmtAge(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-5 * time.Second:    "0s", // clock skew
		0:                   "0s",
		45 * time.Second:    "45s",
		90 * time.Second:    "1m",
		59 * time.Minute:    "59m",
		3 * time.Hour:       "3h",
		47 * time.Hour:      "47h",
		48 * time.Hour:      "2d",
		10 * 24 * time.Hour: "10d",
	} {
		if got := fmtAge(d); got != want {
			t.Errorf("fmtAge(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestFmtDurAndSpan(t *testing.T) {
	if got := fmtDur(-time.Second); got != "0s" {
		t.Errorf("negative duration = %q", got)
	}
	if got := fmtDur(90*time.Second + 400*time.Millisecond); got != "1m30s" {
		t.Errorf("fmtDur = %q", got)
	}
	t0 := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	if got := span(time.Time{}, t0, t0); got != 0 {
		t.Errorf("unstarted span = %v", got)
	}
	if got := span(t0, time.Time{}, t0.Add(time.Minute)); got != time.Minute {
		t.Errorf("running span = %v", got)
	}
	if got := span(t0, t0.Add(-time.Second), t0.Add(time.Minute)); got != time.Minute {
		t.Errorf("end before start should fall back to now: %v", got)
	}
}
