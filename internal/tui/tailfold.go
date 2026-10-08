package tui

import (
	"sort"

	"github.com/rnjn/ghat/internal/tail"
)

// logIndex maps a job's log lines to the lines on screen. End-group
// markers are never shown; lines inside a folded group are hidden behind
// their header. It grows incrementally as lines are appended.
type logIndex struct {
	indexed int
	groupOf []int        // per line: index of the enclosing group header, or -1
	count   map[int]int  // header → member lines
	folded  map[int]bool // folded headers
	open    int          // header of the group being read, or -1
	vis     []int        // visible line indices, ascending
}

func newLogIndex() logIndex {
	return logIndex{count: map[int]int{}, folded: map[int]bool{}, open: -1}
}

// add indexes lines appended since the last call; a shorter log resets.
func (x *logIndex) add(lines []tail.LogLine) {
	if len(lines) < x.indexed {
		*x = newLogIndex()
	}
	for ; x.indexed < len(lines); x.indexed++ {
		i := x.indexed
		g := -1
		switch lines[i].Kind {
		case tail.Group:
			x.open = i
		case tail.EndGroup:
			x.open = -1
		default:
			g = x.open
		}
		x.groupOf = append(x.groupOf, g)
		if g >= 0 {
			x.count[g]++
		}
		if x.visible(i, lines) {
			x.vis = append(x.vis, i)
		}
	}
}

func (x *logIndex) visible(i int, lines []tail.LogLine) bool {
	if lines[i].Kind == tail.EndGroup {
		return false
	}
	g := x.groupOf[i]
	return g < 0 || !x.folded[g]
}

// rebuild recomputes the visible lines after folds change.
func (x *logIndex) rebuild(lines []tail.LogLine) {
	x.vis = x.vis[:0]
	for i := 0; i < x.indexed; i++ {
		if x.visible(i, lines) {
			x.vis = append(x.vis, i)
		}
	}
}

// header is the group header for line i (i itself if it is one), or -1.
func (x *logIndex) header(i int, lines []tail.LogLine) int {
	if lines[i].Kind == tail.Group {
		return i
	}
	return x.groupOf[i]
}

// position is where line i sits among the visible lines (or the nearest
// visible line before it).
func (x *logIndex) position(i int) int {
	p := sort.SearchInts(x.vis, i)
	if p < len(x.vis) && x.vis[p] == i {
		return p
	}
	return max(0, p-1)
}
