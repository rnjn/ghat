package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"ghtui/internal/gh"
)

// runsScreen lists one repo's runs, newest first.
type runsScreen struct {
	repoKey string
	cur     cursor
	selID   int64
}

// NewRuns returns the runs screen for repoKey.
func NewRuns(repoKey string) Screen { return &runsScreen{repoKey: repoKey} }

func (r *runsScreen) Title() string { return r.repoKey }

// OnPop clears the focused run when leaving the repo.
func (r *runsScreen) OnPop(ctx *Context) { ctx.Store.SetFocusRun(0) }

// sync re-finds the selected run by ID after the list changed.
func (r *runsScreen) sync(runs []gh.Run) {
	for i, run := range runs {
		if run.ID == r.selID {
			r.cur.pos = i
			return
		}
	}
	r.cur.clamp(len(runs))
	r.remember(runs)
}

func (r *runsScreen) remember(runs []gh.Run) {
	if len(runs) > 0 {
		r.selID = runs[r.cur.pos].ID
	}
}

func (r *runsScreen) Update(msg tea.Msg, ctx *Context) (Screen, tea.Cmd) {
	runs := ctx.Store.Runs(r.repoKey)
	r.sync(runs)
	k, ok := msg.(tea.KeyPressMsg)
	if !ok || len(runs) == 0 {
		return r, nil
	}
	if r.cur.navKey(k.String(), len(runs), 10) {
		r.remember(runs)
		return r, nil
	}
	run := runs[r.cur.pos]
	if cmd := runKey(ctx, k.String(), run.ID); cmd != nil {
		return r, cmd
	}
	switch k.String() {
	case "enter":
		ctx.Store.SetFocusRun(run.ID)
		ctx.Refresh(fmt.Sprintf("jobs:%d", run.ID))
		return r, push(NewJobs(run))
	case "w":
		ctx.Store.ToggleWatch(run.ID)
	case "o":
		return r, openURL(ctx, run.HTMLURL)
	}
	return r, nil
}

func (r *runsScreen) View(ctx *Context, width, height int) string {
	runs := ctx.Store.Runs(r.repoKey)
	if len(runs) == 0 {
		return fit([]string{"", "  no runs"}, width, height)
	}
	r.sync(runs)
	now := ctx.Now()
	start, end := r.cur.window(len(runs), height-1)
	var rows [][]string
	for _, run := range runs[start:end] {
		watch := " "
		if ctx.Store.Watched(run.ID) {
			watch = styleWarning.Render("w")
		}
		end := run.UpdatedAt
		if run.Status != "completed" {
			end = now
		}
		rows = append(rows, []string{
			glyph(run.Status, run.Conclusion) + " " + state(run.Status, run.Conclusion), watch,
			run.WorkflowName, run.Branch, run.Event, run.Actor, fmtDur(span(run.CreatedAt, end, now)), fmt.Sprintf("#%d", run.Number),
		})
	}
	return fit(table([]string{"STATUS", "W", "WORKFLOW", "BRANCH", "EVENT", "ACTOR", "DURATION", "RUN"}, rows, r.cur.pos-start), width, height)
}

// Resource is what R re-polls.
func (r *runsScreen) Resource() string { return "runs:" + r.repoKey }
