package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"ghtui/internal/gh"
)

// minSplitWidth is the narrowest terminal that shows the steps pane.
const minSplitWidth = 60

// jobsScreen shows a run's jobs (left) and the selected job's steps (right).
type jobsScreen struct {
	run       gh.Run
	jobs      cursor
	steps     cursor
	selID     int64
	stepsPane bool
}

// NewJobs returns the jobs screen for run.
func NewJobs(run gh.Run) Screen { return &jobsScreen{run: run} }

func (j *jobsScreen) Title() string {
	return fmt.Sprintf("%s #%d %s", j.run.RepoKey, j.run.Number, j.run.WorkflowName)
}

// OnPop stops job polling for this run.
func (j *jobsScreen) OnPop(ctx *Context) { ctx.Store.SetFocusRun(0) }

func (j *jobsScreen) sync(jobs []gh.Job) {
	for i, job := range jobs {
		if job.ID == j.selID {
			j.jobs.pos = i
			return
		}
	}
	j.jobs.clamp(len(jobs))
	j.remember(jobs)
}

func (j *jobsScreen) remember(jobs []gh.Job) {
	if len(jobs) > 0 && jobs[j.jobs.pos].ID != j.selID {
		j.selID = jobs[j.jobs.pos].ID
		j.steps = cursor{}
	}
}

func (j *jobsScreen) Update(msg tea.Msg, ctx *Context) (Screen, tea.Cmd) {
	jobs := ctx.Store.Jobs(j.run.ID)
	j.sync(jobs)
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return j, nil
	}
	if cmd := runKey(ctx, k.String(), j.run.ID); cmd != nil {
		return j, cmd
	}
	if cmd := dispatchForRun(ctx, k.String(), j.run.ID); cmd != nil {
		return j, cmd
	}
	if len(jobs) == 0 {
		if k.String() == "o" && j.run.HTMLURL != "" {
			return j, openURL(ctx, j.run.HTMLURL)
		}
		return j, nil
	}
	job := jobs[j.jobs.pos]
	if k.String() == "o" {
		url := job.HTMLURL
		if url == "" {
			url = j.run.HTMLURL
		}
		return j, openURL(ctx, url)
	}
	switch k.String() {
	case "tab":
		j.stepsPane = !j.stepsPane
		return j, nil
	case "right", "l":
		j.stepsPane = true
		return j, nil
	case "left", "h":
		j.stepsPane = false
		return j, nil
	case "enter":
		step := 0
		if j.stepsPane && len(job.Steps) > 0 {
			j.steps.clamp(len(job.Steps))
			step = job.Steps[j.steps.pos].Number
		}
		owner, repo, _ := strings.Cut(j.run.RepoKey, "/")
		ctx.Store.SetTailJob(owner, repo, job.ID)
		t := NewTail(owner, repo, job, step).(*tailScreen)
		t.timestamps = ctx.ShowTimestamps
		return j, push(t)
	}
	if j.stepsPane {
		j.steps.navKey(k.String(), len(job.Steps), 10)
	} else if j.jobs.navKey(k.String(), len(jobs), 10) {
		j.remember(jobs)
	}
	return j, nil
}

func (j *jobsScreen) View(ctx *Context, width, height int) string {
	jobs := ctx.Store.Jobs(j.run.ID)
	if len(jobs) == 0 {
		if _, ok := ctx.Store.Run(j.run.ID); !ok {
			return fit([]string{"", "  run no longer tracked (its repo left the board)"}, width, height)
		}
		return fit([]string{"", "  loading jobs…"}, width, height)
	}
	j.sync(jobs)
	now := ctx.Now()
	split := width >= minSplitWidth
	leftW := width
	if split {
		leftW = width * 2 / 5
	}

	start, end := j.jobs.window(len(jobs), height-1)
	var rows [][]string
	for _, job := range jobs[start:end] {
		rows = append(rows, []string{glyph(job.Status, job.Conclusion), job.Name, fmtDur(span(job.StartedAt, job.CompletedAt, now))})
	}
	left := table([]string{" ", "JOB", "DURATION"}, rows, j.jobs.pos-start)
	if !split {
		return fit(left, width, height)
	}

	steps := jobs[j.jobs.pos].Steps
	sStart, sEnd := j.steps.window(len(steps), height-1)
	rows = nil
	for _, st := range steps[sStart:sEnd] {
		dur := ""
		if !st.StartedAt.IsZero() {
			dur = fmtDur(span(st.StartedAt, st.CompletedAt, now))
		}
		rows = append(rows, []string{glyph(st.Status, st.Conclusion), st.Name, dur})
	}
	sel := -1
	if j.stepsPane {
		sel = j.steps.pos - sStart
	}
	right := table([]string{" ", "STEP", "DURATION"}, rows, sel)

	rightW := width - leftW - 3
	out := make([]string, height)
	for i := range out {
		var l, r string
		if i < len(left) {
			l = truncate(left[i], leftW)
		}
		if i < len(right) {
			r = truncate(right[i], rightW)
		}
		out[i] = l + strings.Repeat(" ", max(0, leftW-ansi.StringWidth(l))) + styleDim.Render(" │ ") + r
	}
	return fit(out, width, height)
}

// Resource is what R re-polls.
func (j *jobsScreen) Resource() string { return fmt.Sprintf("jobs:%d", j.run.ID) }
