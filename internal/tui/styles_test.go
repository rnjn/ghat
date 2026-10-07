package tui

import (
	"strings"
	"testing"
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
