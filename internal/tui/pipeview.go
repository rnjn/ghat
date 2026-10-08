package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/workflow"
)

// viewMode picks how the pipeline view draws a run.
type viewMode int

const (
	modeDAG viewMode = iota
	modeTimeline
)

// graphLoaded carries a workflow's job specs for a pipeline view.
type graphLoaded struct {
	view  *pipeView
	specs []workflow.Job
	err   error
}

// pipeView draws one run as a dependency graph or a timeline.
type pipeView struct {
	run     gh.Run
	mode    viewMode
	selID   int64
	specs   []workflow.Job
	loading bool
	loadErr error
	restore int64 // focus run to restore on pop
	offX    int   // graph scroll
	offY    int
}

// pipelineViewKey opens the pipeline view for runID when k is v. A run not
// yet focused (opened from Runs) is focused so its jobs get polled.
func pipelineViewKey(ctx *Context, k string, runID int64) tea.Cmd {
	if k != "v" {
		return nil
	}
	run, ok := ctx.Store.Run(runID)
	if !ok {
		return fail(errors.New("run is no longer listed"))
	}
	restore := ctx.Store.FocusRun()
	if restore != runID {
		ctx.Store.SetFocusRun(runID)
		ctx.Refresh(fmt.Sprintf("jobs:%d", runID))
	}
	v := &pipeView{run: run, restore: restore}
	cmds := []tea.Cmd{push(v)}
	if ctx.Actions != nil && run.WorkflowID != 0 && run.HeadSHA != "" {
		v.loading = true
		acts := ctx.Actions
		owner, repo := splitKey(run.RepoKey)
		cmds = append(cmds, func() tea.Msg {
			specs, err := acts.JobGraph(context.Background(), owner, repo, run.WorkflowID, run.HeadSHA)
			return graphLoaded{view: v, specs: specs, err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (v *pipeView) Title() string {
	mode := "Graph"
	if v.mode == modeTimeline {
		mode = "Timeline"
	}
	return fmt.Sprintf("%s · %s #%d %s", mode, v.run.RepoKey, v.run.Number, v.run.WorkflowName)
}

// OnPop restores the focus the view found.
func (v *pipeView) OnPop(ctx *Context) { ctx.Store.SetFocusRun(v.restore) }

// Resource is what R re-polls.
func (v *pipeView) Resource() string { return fmt.Sprintf("jobs:%d", v.run.ID) }

func (v *pipeView) graph(ctx *Context) graph {
	return buildGraph(ctx.Store.Jobs(v.run.ID), v.specs, ctx.Now(), v.active(ctx))
}

// active reports whether the run is still going, by its latest state.
func (v *pipeView) active(ctx *Context) bool {
	run := v.run
	if latest, ok := ctx.Store.Run(run.ID); ok {
		run = latest
	}
	return run.Status != "completed"
}

// order lists node indexes column by column, the j/k order.
func (g *graph) order() []int {
	var out []int
	for _, c := range g.cols {
		out = append(out, c...)
	}
	return out
}

// selected returns the index of the selected node, defaulting to the first.
func (v *pipeView) selected(g *graph) int {
	for _, i := range g.order() {
		if g.nodes[i].job.ID == v.selID {
			return i
		}
	}
	if len(g.cols) > 0 && len(g.cols[0]) > 0 {
		v.selID = g.nodes[g.cols[0][0]].job.ID
		return g.cols[0][0]
	}
	return -1
}

func (v *pipeView) Update(msg tea.Msg, ctx *Context) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case graphLoaded:
		if m.view == v {
			v.loading, v.specs, v.loadErr = false, m.specs, m.err
		}
	case tea.KeyPressMsg:
		return v, v.key(m.String(), ctx)
	}
	return v, nil
}

func (v *pipeView) key(k string, ctx *Context) tea.Cmd {
	if cmd := runKey(ctx, k, v.run.ID); cmd != nil {
		return cmd
	}
	if cmd := dispatchForRun(ctx, k, v.run.ID); cmd != nil {
		return cmd
	}
	if cmd := commitKey(ctx, k, v.run.ID); cmd != nil {
		return cmd
	}
	if k == "tab" {
		v.mode ^= 1
		return nil
	}
	g := v.graph(ctx)
	sel := v.selected(&g)
	if sel < 0 {
		if (k == "o" || k == "O") && v.run.HTMLURL != "" {
			return linkKey(ctx, k, v.run.HTMLURL)
		}
		return nil
	}
	n := g.nodes[sel]
	job := n.job
	switch k {
	case "o", "O":
		url := job.HTMLURL
		if url == "" {
			url = v.run.HTMLURL
		}
		return linkKey(ctx, k, url)
	case "enter":
		if n.placeholder {
			return fail(errors.New("job not created yet: its needs are still running"))
		}
		owner, repo := splitKey(v.run.RepoKey)
		ctx.Store.SetTailJob(owner, repo, job.ID)
		t := NewTail(owner, repo, job, 0).(*tailScreen)
		t.timestamps = ctx.ShowTimestamps
		return push(t)
	}
	v.move(k, &g, sel)
	return nil
}

// move applies a navigation key: j/k walk the jobs column by column (or by
// start time on the timeline), h/l jump to the neighbouring column.
func (v *pipeView) move(k string, g *graph, sel int) {
	order := g.order()
	if v.mode == modeTimeline {
		order = timelineOrder(g)
	}
	pos := 0
	for i, n := range order {
		if n == sel {
			pos = i
		}
	}
	col, row := g.nodes[sel].col, 0
	for i, n := range g.cols[col] {
		if n == sel {
			row = i
		}
	}
	switch k {
	case "j", "down":
		pos++
	case "k", "up":
		pos--
	case "g", "home":
		pos = 0
	case "G", "end":
		pos = len(order) - 1
	case "l", "right":
		if v.mode == modeDAG && col+1 < len(g.cols) {
			v.selID = g.nodes[g.cols[col+1][min(row, len(g.cols[col+1])-1)]].job.ID
		}
		return
	case "h", "left":
		if v.mode == modeDAG && col > 0 {
			v.selID = g.nodes[g.cols[col-1][min(row, len(g.cols[col-1])-1)]].job.ID
		}
		return
	default:
		return
	}
	pos = max(0, min(pos, len(order)-1))
	v.selID = g.nodes[order[pos]].job.ID
}

func (v *pipeView) View(ctx *Context, width, height int) string {
	jobs := ctx.Store.Jobs(v.run.ID)
	if len(jobs) == 0 {
		if _, ok := ctx.Store.Run(v.run.ID); !ok {
			return fit([]string{"", "  run no longer tracked (its repo left the board)"}, width, height)
		}
		return fit([]string{"", "  loading jobs…"}, width, height)
	}
	if width <= 0 || height <= 0 {
		return ""
	}
	g := v.graph(ctx)
	sel := v.selected(&g)
	var body []string
	footer := " tab: timeline · ←/→ columns · enter: log"
	if v.mode == modeTimeline {
		body = v.viewTimeline(ctx, &g, sel, width, height-1)
		footer = " tab: graph · enter: log"
	} else {
		body = v.viewDAG(ctx, &g, sel, width, height-1)
	}
	for len(body) < height-1 {
		body = append(body, "")
	}
	return fit(append(body, styleDim.Render(footer)), width, height)
}

// Graph layout constants: box height, row pitch, gutter between columns.
const (
	boxH     = 3
	rowPitch = 4
	gutterW  = 5
	maxNameW = 24
)

// statusStyle colours a job's box by its state.
func statusStyle(job gh.Job, selected bool) lipgloss.Style {
	st := styleDim
	switch {
	case job.Status == "in_progress":
		st = lipgloss.NewStyle().Foreground(colorYellow)
	case job.Status == "completed" && job.Conclusion == "success":
		st = lipgloss.NewStyle().Foreground(colorGreen)
	case job.Status == "completed" && isFailure(job.Conclusion):
		st = styleError
	}
	if selected {
		st = st.Bold(true)
	}
	return st
}

// nodeDur is a job's duration so far, or "" for one not created yet.
func nodeDur(n node, now time.Time) string {
	if n.placeholder {
		return ""
	}
	return fmtDur(span(n.job.StartedAt, n.job.CompletedAt, now))
}

// jobLabel is "name" capped to maxNameW, bold on the critical path.
func jobLabel(n node) string {
	name := truncate(n.job.Name, maxNameW)
	if n.critical {
		return styleHeader.Render(name)
	}
	return name
}

func (v *pipeView) viewDAG(ctx *Context, g *graph, sel, width, height int) []string {
	now := ctx.Now()
	// Column widths and positions.
	boxW := make([]int, len(g.cols))
	x := make([]int, len(g.cols))
	rows := 0
	for c, col := range g.cols {
		rows = max(rows, len(col))
		for _, i := range col {
			n := g.nodes[i]
			w := 2 + min(ansi.StringWidth(n.job.Name), maxNameW) + 1 + len(nodeDur(n, now))
			boxW[c] = max(boxW[c], w+4)
		}
		if c > 0 {
			x[c] = x[c-1] + boxW[c-1] + gutterW
		} else {
			x[c] = 1
		}
	}
	rowOf := func(i int) int {
		for r, n := range g.cols[g.nodes[i].col] {
			if n == i {
				return r
			}
		}
		return 0
	}
	y := func(i int) int { return 1 + rowOf(i)*rowPitch }
	last := len(g.cols) - 1
	cv := newCanvas(x[last]+boxW[last]+1, 1+rows*rowPitch)
	for i, n := range g.nodes {
		for _, u := range n.needs {
			uc := g.nodes[u].col
			cv.edge(x[uc]+boxW[uc]-1, y(u)+1, x[n.col], y(i)+1)
		}
	}
	for i, n := range g.nodes {
		st := statusStyle(n.job, i == sel)
		cv.box(x[n.col], y(i), boxW[n.col], boxH, &st)
		dur := nodeDur(n, now)
		inner := boxW[n.col] - 4
		label := jobLabel(n)
		pad := inner - 2 - ansi.StringWidth(label) - len(dur)
		text := glyph(n.job.Status, n.job.Conclusion) + " " + label + strings.Repeat(" ", max(1, pad)) + dur
		if i == sel {
			text = styleCursor.Render(ansi.Strip(text))
		}
		cv.put(x[n.col]+2, y(i)+1, " ", nil) // clear the pad cell
		cv.putStyled(x[n.col]+2, y(i)+1, text)
	}
	// Scroll to keep the selection in view.
	if sel >= 0 {
		c := g.nodes[sel].col
		v.offX = min(v.offX, x[c]-1)
		v.offX = max(v.offX, x[c]+boxW[c]+1-width)
		v.offY = min(v.offY, y(sel)-1)
		v.offY = max(v.offY, y(sel)+boxH+1-height)
	}
	v.offX = max(0, min(v.offX, cv.w-width))
	v.offY = max(0, min(v.offY, cv.h-height))
	all := cv.render()
	var out []string
	for r := v.offY; r < min(cv.h, v.offY+height); r++ {
		out = append(out, ansi.Cut(all[r], v.offX, v.offX+width))
	}
	return out
}
