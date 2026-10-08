package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/store"
)

// headed is implemented by screens that describe themselves in the header:
// a heading for the title line and up to two lines of stats.
type headed interface {
	Heading(ctx *Context) string
	Stats(ctx *Context) []string
}

// minRowsForStats is the shortest terminal that gets the stats lines.
const minRowsForStats = 15

// header renders the title line, and on tall enough terminals the stats
// and a rule. It never exceeds four lines.
func (m Model) header() []string {
	heading, stats := m.top().Title(), []string(nil)
	if h, ok := m.top().(headed); ok {
		heading, stats = h.Heading(m.ctx), h.Stats(m.ctx)
	}
	now := m.ctx.Now()
	right := now.Local().Format("15:04")
	if q := m.status.quota(now); q != "" {
		right = q + " · " + right
	}
	lines := []string{titleBar(" ghat ▸ "+heading, right+" ", m.width)}
	if m.height < minRowsForStats {
		return lines
	}
	for _, s := range stats[:min(2, len(stats))] {
		lines = append(lines, truncate(" "+s, m.width))
	}
	return append(lines, styleDim.Render(strings.Repeat("─", m.width)))
}

// titleBar puts left and right on one full-width bar.
func titleBar(left, right string, width int) string {
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	line := left + strings.Repeat(" ", max(1, gap)) + right
	if gap < 1 {
		line = truncate(left, width)
		line += strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
	}
	return styleTitle.Render(line)
}

func pct(n, of int) string {
	if of == 0 {
		return "–"
	}
	return fmt.Sprintf("%d%%", n*100/of)
}

// runStats counts a set of runs.
type runStats struct {
	total, active, running, queued, finished, success, failed int
}

func countRuns(runs []gh.Run) runStats {
	var s runStats
	for _, r := range runs {
		s.total++
		switch {
		case r.Status == "in_progress":
			s.active++
			s.running++
		case store.IsActive(r.Status):
			s.active++
			s.queued++
		case r.Status == "completed":
			s.finished++
			if r.Conclusion == "success" {
				s.success++
			}
			if isFailure(r.Conclusion) {
				s.failed++
			}
		}
	}
	return s
}

// Board.

func (b *board) Heading(*Context) string { return "Repositories" }

func (b *board) Stats(ctx *Context) []string {
	repos := ctx.Store.Repos()
	unavailable := 0
	var runs []gh.Run
	for _, r := range repos {
		if r.Unavailable {
			unavailable++
		}
		runs = append(runs, ctx.Store.Runs(r.Repo.Key())...)
	}
	s := countRuns(runs)
	return []string{
		fmt.Sprintf("%d repos · %d unavailable · %d active (%d running, %d queued)", len(repos), unavailable, s.active, s.running, s.queued),
		fmt.Sprintf("%d runs loaded · %s success · %d failed · %d watched", s.total, pct(s.success, s.finished), s.failed, len(ctx.Store.WatchedRuns())),
	}
}

// Runs.

func (r *runsScreen) Heading(*Context) string {
	h := "Runs · " + r.repoKey
	if r.filter != "" {
		h += " · /" + r.filter
	}
	return h
}

func (r *runsScreen) Stats(ctx *Context) []string {
	runs := ctx.Store.Runs(r.repoKey)
	s := countRuns(runs)
	var durs []time.Duration
	lastFail, watched := "no failures", 0
	now := ctx.Now()
	for _, run := range runs {
		if run.Status == "completed" {
			durs = append(durs, span(run.CreatedAt, run.UpdatedAt, now))
			if isFailure(run.Conclusion) && lastFail == "no failures" {
				lastFail = "last failure " + fmtAge(now.Sub(run.UpdatedAt)) + " ago"
			}
		}
		if ctx.Store.Watched(run.ID) {
			watched++
		}
	}
	median := "–"
	if len(durs) > 0 {
		slices.Sort(durs)
		median = fmtDur(durs[len(durs)/2])
	}
	line2 := fmt.Sprintf("%s · %d watched", lastFail, watched)
	if rs, ok := ctx.Store.Repo(r.repoKey); ok && rs.Repo.DefaultBranch != "" {
		line2 += " · default branch " + rs.Repo.DefaultBranch
	}
	return []string{
		fmt.Sprintf("%d runs · %d active · %s success of %d finished · median %s", s.total, s.active, pct(s.success, s.finished), s.finished, median),
		line2,
	}
}

// Pipeline (Jobs).

func (j *jobsScreen) Heading(*Context) string {
	return fmt.Sprintf("Pipeline · %s #%d %s", j.run.RepoKey, j.run.Number, j.run.WorkflowName)
}

// runLine is the header's first stats line for a run: state, branch,
// event, actor and elapsed time.
func runLine(ctx *Context, run gh.Run) string {
	if latest, ok := ctx.Store.Run(run.ID); ok {
		run = latest
	}
	now := ctx.Now()
	end := run.UpdatedAt
	if run.Status != "completed" {
		end = now
	}
	return fmt.Sprintf("%s %s · %s · %s · %s · %s", glyph(run.Status, run.Conclusion), state(run.Status, run.Conclusion),
		run.Branch, run.Event, run.Actor, fmtDur(span(run.CreatedAt, end, now)))
}

func (j *jobsScreen) Stats(ctx *Context) []string {
	run := j.run
	if latest, ok := ctx.Store.Run(run.ID); ok {
		run = latest
	}
	jobs := ctx.Store.Jobs(run.ID)
	done, failed, running := 0, 0, ""
	for _, job := range jobs {
		if job.Status == "completed" {
			done++
			if isFailure(job.Conclusion) {
				failed++
			}
		}
		if job.Status == "in_progress" && running == "" {
			running = "running " + job.Name
			for _, st := range job.Steps {
				if st.Status == "in_progress" {
					running += " › " + st.Name
					break
				}
			}
		}
	}
	line2 := fmt.Sprintf("jobs %d/%d done · %d failed", done, len(jobs), failed)
	if sha := run.ShortSHA(); sha != "" {
		line2 = hyperlink(run.CommitURL(), sha) + " " + run.CommitMessage + " · " + line2
	}
	if running != "" {
		line2 += " · " + running
	}
	return []string{runLine(ctx, run), line2}
}

// Pipeline view (Graph / Timeline).

func (v *pipeView) Heading(*Context) string { return v.Title() }

func (v *pipeView) Stats(ctx *Context) []string {
	g := v.graph(ctx)
	line2 := fmt.Sprintf("%d jobs · %d columns", len(g.nodes), len(g.cols))
	if g.pending > 0 {
		line2 = fmt.Sprintf("%d jobs (%d not created yet) · %d columns", len(g.nodes), g.pending, len(g.cols))
	}
	if len(g.critical) > 0 {
		names := make([]string, len(g.critical))
		for i, n := range g.critical {
			names[i] = g.nodes[n].job.Name
		}
		last := g.nodes[g.critical[len(g.critical)-1]].job
		line2 += fmt.Sprintf(" · critical path %s (%s)", strings.Join(names, " › "), fmtDur(span(v.run.CreatedAt, jobEnd(last, ctx.Now()), ctx.Now())))
	}
	switch {
	case v.loading:
		line2 += " · loading dependencies…"
	case v.loadErr != nil:
		line2 += " · needs from timing (" + v.loadErr.Error() + ")"
	case g.inferred:
		line2 += " · needs from timing"
	}
	return []string{runLine(ctx, v.run), line2}
}

// Logs (Tail).

func (t *tailScreen) Heading(*Context) string { return "Logs · " + t.job.Name }

func (t *tailScreen) Stats(ctx *Context) []string {
	now := ctx.Now()
	done, running := 0, ""
	for _, st := range t.job.Steps {
		if st.Status == "completed" {
			done++
		}
		if st.Status == "in_progress" {
			running = " · " + st.Name + " " + fmtDur(span(st.StartedAt, st.CompletedAt, now))
		}
	}
	line1 := fmt.Sprintf("%s %s · steps %d/%d done%s", glyph(t.job.Status, t.job.Conclusion),
		state(t.job.Status, t.job.Conclusion), done, len(t.job.Steps), running)
	line2 := fmt.Sprintf("%d lines", t.ix.indexed)
	if n := len(t.ix.folded); n > 0 {
		folded := 0
		for _, f := range t.ix.folded {
			if f {
				folded++
			}
		}
		line2 += fmt.Sprintf(" · %d folded", folded)
	}
	if st := t.search.status(); st != "" && !t.search.open {
		line2 += " · " + st
	}
	return []string{line1, line2}
}

// Dispatch.

func (p *dispatchPicker) Heading(*Context) string { return "Dispatch · " + p.repoKey }

func (p *dispatchPicker) Stats(*Context) []string {
	return []string{"workflows with a workflow_dispatch trigger · ref " + p.ref}
}

func (f *dispatchForm) Heading(*Context) string {
	return "Dispatch · " + f.repoKey + " · " + f.wf.Workflow.Name
}

func (f *dispatchForm) Stats(*Context) []string {
	return []string{fmt.Sprintf("%d inputs · %s", len(f.wf.Inputs), f.wf.Workflow.Path)}
}
