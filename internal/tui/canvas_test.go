package tui

import (
	"strings"
	"testing"
)

func plainRows(rows []string) []string {
	for i, r := range rows {
		rows[i] = plain(r)
	}
	return rows
}

func TestCanvasLinesMergeIntoJunctions(t *testing.T) {
	c := newCanvas(7, 5)
	c.hline(2, 0, 6)
	c.vline(3, 0, 4)
	c.hline(0, 3, 6) // starts on the vertical: ╭
	c.vline(5, 2, 4) // starts on the horizontal: ┬
	got := strings.Join(plainRows(c.render()), "\n")
	want := strings.Join([]string{
		"   ╭───",
		"   │   ",
		"───┼─┬─",
		"   │ │ ",
		"   │ │ ",
	}, "\n")
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestCanvasEdgeAndBoxPaintOrder(t *testing.T) {
	c := newCanvas(20, 7)
	c.edge(4, 1, 14, 5) // from the right of a box at row 1 to the left of one at row 5
	c.box(0, 0, 5, 3, nil)
	c.put(1, 1, "a", nil)
	c.box(14, 4, 5, 3, nil)
	c.put(15, 5, "b", nil)
	c.box(9, 0, 3, 3, nil) // sits on the horizontal line and hides it
	got := strings.Join(plainRows(c.render()), "\n")
	want := strings.Join([]string{
		"┌───┐    ┌─┐        ",
		"│a  │────│ │╮       ",
		"└───┘    └─┘│       ",
		"            │       ",
		"            │ ┌───┐ ",
		"            ╰▶│b  │ ",
		"              └───┘ ",
	}, "\n")
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestCanvasEdgeSameRowAndUpward(t *testing.T) {
	c := newCanvas(12, 3)
	c.edge(1, 1, 9, 1)
	c.edge(1, 2, 9, 0)
	got := strings.Join(plainRows(c.render()), "\n")
	want := strings.Join([]string{
		"       ╭▶   ",
		" ──────┼▶   ",
		" ──────╯    ",
	}, "\n")
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestCanvasIgnoresOutOfBounds(t *testing.T) {
	c := newCanvas(3, 2)
	c.hline(5, -1, 9)
	c.vline(-2, 0, 9)
	c.box(-1, -1, 10, 10, nil)
	c.put(2, 1, "xyz", nil)
	if rows := plainRows(c.render()); len(rows) != 2 || rows[1] != "  x" {
		t.Fatalf("rows %q", rows)
	}
}
