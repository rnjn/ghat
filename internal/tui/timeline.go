package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rnjn/ghat/internal/gh"
)

// timelineOrder lists node indexes by start time, unstarted jobs last.
func timelineOrder(g *graph) []int {
	order := make([]int, len(g.nodes))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		sa, sb := g.nodes[a].job.StartedAt, g.nodes[b].job.StartedAt
		switch {
		case sa.IsZero() && sb.IsZero():
			return 0
		case sa.IsZero():
			return 1
		case sb.IsZero():
			return -1
		}
		return sa.Compare(sb)
	})
	return order
}

// tickSteps are the axis intervals to choose from.
var tickSteps = []time.Duration{time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second,
	time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 30 * time.Minute,
	time.Hour, 2 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour}

// tickLabel is a short duration: 30s, 5m, 1m10s, 1h, 1h30m.
func tickLabel(d time.Duration) string {
	if d == 0 {
		return "0"
	}
	h, m, sec := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	var b strings.Builder
	if h > 0 {
		fmt.Fprintf(&b, "%dh", h)
	}
	if m > 0 || (h > 0 && sec > 0) {
		fmt.Fprintf(&b, "%dm", m)
	}
	if sec > 0 {
		fmt.Fprintf(&b, "%ds", sec)
	}
	return b.String()
}

// axis maps times onto a bar of w cells.
type axis struct {
	t0    time.Time
	total time.Duration
	w     int
}

func (a axis) cell(t time.Time) int {
	if t.IsZero() || a.total <= 0 {
		return 0
	}
	return max(0, min(a.w, int(int64(t.Sub(a.t0))*int64(a.w)/int64(a.total))))
}

// bar draws wait (░) from created to started and run (█) from started to
// end, at least one cell long once started.
func (a axis) bar(created, started, end time.Time, st lipgloss.Style) string {
	if started.IsZero() {
		return ""
	}
	s := a.cell(started)
	lead := s
	if !created.IsZero() && created.Before(started) {
		lead = a.cell(created)
	}
	out := strings.Repeat(" ", lead)
	if s > lead {
		out += styleDim.Render(strings.Repeat("░", s-lead))
	}
	e := min(a.w, max(s+1, a.cell(end)))
	return out + st.Render(strings.Repeat("█", max(0, e-s)))
}

// labels renders the tick row and the rule row.
func (a axis) labels() (ticks, rule string) {
	step := tickSteps[len(tickSteps)-1]
	for _, s := range tickSteps {
		if a.total <= 0 || int64(s)*int64(a.w)/int64(a.total) >= 7 {
			step = s
			break
		}
	}
	t := make([]rune, a.w)
	r := make([]rune, a.w)
	for i := range t {
		t[i], r[i] = ' ', '─'
	}
	for d := time.Duration(0); d <= a.total; d += step {
		x := a.cell(a.t0.Add(d))
		if x >= a.w {
			break
		}
		r[x] = '┬'
		label := []rune(tickLabel(d))
		if x+len(label) > a.w {
			continue
		}
		copy(t[x:], label)
	}
	return styleDim.Render(string(t)), styleDim.Render(string(r))
}

// timelineRow is one line of the timeline: a job or a step of the
// selected job.
type timelineRow struct {
	node int // node index, -1 for a step row
	text string
}

func (v *pipeView) viewTimeline(ctx *Context, g *graph, sel, width, height int) []string {
	now := ctx.Now()
	run := v.run
	if latest, ok := ctx.Store.Run(run.ID); ok {
		run = latest
	}
	labelW := 0
	for _, n := range g.nodes {
		labelW = max(labelW, min(ansi.StringWidth(n.job.Name), maxNameW)+4) // "› " and glyph
	}
	if sel >= 0 {
		for _, st := range g.nodes[sel].job.Steps {
			labelW = max(labelW, min(ansi.StringWidth(st.Name), maxNameW)+6) // indented under the job
		}
	}
	const durW = 9
	barW := width - labelW - durW - 2
	end := run.UpdatedAt
	if run.Status != "completed" {
		end = now
	}
	for _, n := range g.nodes {
		if e := jobEnd(n.job, now); e.After(end) {
			end = e
		}
	}
	ax := axis{t0: run.CreatedAt, total: end.Sub(run.CreatedAt), w: max(0, barW)}
	line := func(prefix, label string, w int, bar, dur string) string {
		l := prefix + label
		return l + strings.Repeat(" ", max(0, w-ansi.StringWidth(l))) + " " + bar + strings.Repeat(" ", max(0, barW-ansi.StringWidth(bar))) + " " + dur
	}
	var rows []timelineRow
	selRow, lastStep := 0, 0
	for _, i := range timelineOrder(g) {
		n := g.nodes[i]
		j := n.job
		prefix, label := "  ", glyph(j.Status, j.Conclusion)+" "+jobLabel(n)
		if i == sel {
			prefix, label = "› ", styleCursor.Render(ansi.Strip(label))
			selRow = len(rows)
		}
		dur := fmtDur(span(j.StartedAt, j.CompletedAt, now))
		if j.StartedAt.IsZero() {
			dur = styleDim.Render(state(j.Status, j.Conclusion))
		}
		rows = append(rows, timelineRow{node: i, text: line(prefix, label, labelW, ax.bar(j.CreatedAt, j.StartedAt, jobEnd(j, now), statusStyle(j, false)), dur)})
		if i != sel {
			continue
		}
		for _, st := range j.Steps {
			sdur := ""
			if !st.StartedAt.IsZero() {
				sdur = fmtDur(span(st.StartedAt, st.CompletedAt, now))
			}
			sj := gh.Job{Status: st.Status, Conclusion: st.Conclusion}
			bar := ax.bar(time.Time{}, st.StartedAt, stepEnd(st, now), statusStyle(sj, false))
			label := glyph(st.Status, st.Conclusion) + " " + styleDim.Render(truncate(st.Name, labelW-6))
			rows = append(rows, timelineRow{node: -1, text: line("    ", label, labelW, bar, styleDim.Render(sdur))})
			lastStep = len(rows) - 1
		}
	}
	ticks, rule := ax.labels()
	head := []string{strings.Repeat(" ", labelW+1) + ticks, strings.Repeat(" ", labelW+1) + rule}
	bodyH := max(0, height-len(head))
	// Keep the selected job, and as many of its steps as fit, in view.
	off := v.offY
	if selRow < off {
		off = selRow
	}
	if selRow >= off+bodyH {
		off = selRow - bodyH + 1
	}
	if lastStep > selRow && lastStep >= off+bodyH {
		off = min(selRow, lastStep-bodyH+1)
	}
	off = max(0, min(off, len(rows)-bodyH))
	v.offY = off
	out := head
	for _, r := range rows[off:min(len(rows), off+bodyH)] {
		out = append(out, r.text)
	}
	return out
}

// stepEnd is when a step stopped, now for a running one.
func stepEnd(st gh.Step, now time.Time) time.Time {
	if st.Status == "completed" {
		return st.CompletedAt
	}
	return now
}
