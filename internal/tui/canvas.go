package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Line directions a cell takes part in.
const (
	dirUp uint8 = 1 << iota
	dirDown
	dirLeft
	dirRight
)

// lineRunes maps a direction mask to the box-drawing rune that joins them.
var lineRunes = [16]rune{
	' ', '│', '│', '│', '─', '╯', '╮', '┤',
	'─', '╰', '╭', '├', '─', '┴', '┬', '┼',
}

type cell struct {
	text  string // a painted character; "" when the cell is a line or blank
	mask  uint8
	style *lipgloss.Style
}

// canvas is a grid of cells for drawing boxes and the lines between them.
// Lines merge where they meet; text paints over lines.
type canvas struct {
	w, h   int
	cells  [][]cell
	styled []styledRun
}

// styledRun is a pre-styled string spliced over the cells it covers.
type styledRun struct {
	x, y, w int
	s       string
}

func newCanvas(w, h int) *canvas {
	c := &canvas{w: max(0, w), h: max(0, h), cells: make([][]cell, max(0, h))}
	for y := range c.cells {
		c.cells[y] = make([]cell, c.w)
	}
	return c
}

func (c *canvas) at(x, y int) *cell {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return nil
	}
	return &c.cells[y][x]
}

func (c *canvas) mark(x, y int, dir uint8) {
	if cl := c.at(x, y); cl != nil {
		cl.mask |= dir
		cl.text = ""
	}
}

// hline draws a horizontal line on row y from x1 to x2 inclusive.
func (c *canvas) hline(y, x1, x2 int) {
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if x1 == x2 {
		return
	}
	c.mark(x1, y, dirRight)
	for x := x1 + 1; x < x2; x++ {
		c.mark(x, y, dirLeft|dirRight)
	}
	c.mark(x2, y, dirLeft)
}

// vline draws a vertical line in column x from y1 to y2 inclusive.
func (c *canvas) vline(x, y1, y2 int) {
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	if y1 == y2 {
		return
	}
	c.mark(x, y1, dirDown)
	for y := y1 + 1; y < y2; y++ {
		c.mark(x, y, dirUp|dirDown)
	}
	c.mark(x, y2, dirUp)
}

// put paints s from (x, y), one cell per rune.
func (c *canvas) put(x, y int, s string, style *lipgloss.Style) {
	for i, r := range []rune(s) {
		if cl := c.at(x+i, y); cl != nil {
			*cl = cell{text: string(r), style: style}
		}
	}
}

// box paints a w×h bordered box with its top left at (x, y), clearing the
// inside.
func (c *canvas) box(x, y, w, h int, style *lipgloss.Style) {
	if w < 2 || h < 2 {
		return
	}
	mid := strings.Repeat("─", w-2)
	c.put(x, y, "┌"+mid+"┐", style)
	for row := y + 1; row < y+h-1; row++ {
		c.put(x, row, "│"+strings.Repeat(" ", w-2)+"│", style)
	}
	c.put(x, y+h-1, "└"+mid+"┘", style)
}

// edge draws a dependency arrow from (ux, uy), the right edge of one box,
// to (vx, vy), the left edge of another: across, down or up through the
// column two cells before vx, then into vx with an arrowhead.
func (c *canvas) edge(ux, uy, vx, vy int) {
	gx := vx - 2
	c.hline(uy, ux, gx)
	c.vline(gx, uy, vy)
	c.hline(vy, gx, vx-1)
	c.put(vx-1, vy, "▶", &styleDim)
}

// render returns the rows as strings. Line cells are dim; painted cells
// keep their style.
func (c *canvas) render() []string {
	out := make([]string, c.h)
	for y, row := range c.cells {
		var b strings.Builder
		var run strings.Builder
		var cur *lipgloss.Style
		flush := func() {
			if run.Len() == 0 {
				return
			}
			if cur != nil {
				b.WriteString(cur.Render(run.String()))
			} else {
				b.WriteString(run.String())
			}
			run.Reset()
		}
		runs := map[int]styledRun{}
		for _, sr := range c.styled {
			if sr.y == y {
				runs[sr.x] = sr
			}
		}
		for x := 0; x < len(row); x++ {
			if sr, ok := runs[x]; ok {
				flush()
				cur = nil
				b.WriteString(sr.s)
				x += sr.w - 1
				continue
			}
			cl := row[x]
			text, style := cl.text, cl.style
			if text == "" {
				text = string(lineRunes[cl.mask&15])
				if cl.mask != 0 {
					style = &styleDim
				}
			}
			if style != cur {
				flush()
				cur = style
			}
			run.WriteString(text)
		}
		flush()
		out[y] = b.String()
	}
	return out
}

// putStyled lays an already styled string over the cells from (x, y); it
// is emitted as is when the row renders. It must fit the canvas.
func (c *canvas) putStyled(x, y int, s string) {
	w := ansi.StringWidth(s)
	if y < 0 || y >= c.h || x < 0 || x+w > c.w {
		return
	}
	c.put(x, y, strings.Repeat(" ", w), nil)
	c.styled = append(c.styled, styledRun{x: x, y: y, s: s, w: w})
}
