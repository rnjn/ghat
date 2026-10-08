package tui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghat/internal/gh"
)

// runsScreen lists one repo's runs, newest first.
type runsScreen struct {
	repoKey   string
	cur       cursor
	selID     int64
	filtering bool // filter line open
	input     textinput.Model
	filter    string // applied filter, lower-cased
}

// NewRuns returns the runs screen for repoKey.
func NewRuns(repoKey string) Screen { return &runsScreen{repoKey: repoKey} }

func (r *runsScreen) Title() string {
	if r.filter != "" {
		return r.repoKey + " · /" + r.filter
	}
	return r.repoKey
}

// CapturesKeys sends typed keys to the filter line while it is open.
func (r *runsScreen) CapturesKeys() bool { return r.filtering }

// HandleEsc closes the filter line or clears the filter before leaving.
func (r *runsScreen) HandleEsc() bool {
	if r.filtering || r.filter != "" {
		r.filtering, r.filter = false, ""
		return true
	}
	return false
}

// visible is the repo's runs that match the filter (branch or status).
func (r *runsScreen) visible(ctx *Context) []gh.Run {
	runs := ctx.Store.Runs(r.repoKey)
	if r.filter == "" {
		return runs
	}
	var out []gh.Run
	for _, run := range runs {
		if strings.Contains(strings.ToLower(run.Branch), r.filter) ||
			strings.Contains(strings.ToLower(state(run.Status, run.Conclusion)), r.filter) ||
			strings.Contains(strings.ToLower(run.Status), r.filter) {
			out = append(out, run)
		}
	}
	return out
}

// filterKey handles keys while the filter line is open, and /.
func (r *runsScreen) filterKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if r.filtering {
		if msg.String() == "enter" {
			r.filtering = false
			return true, nil
		}
		var cmd tea.Cmd
		r.input, cmd = r.input.Update(msg)
		r.filter = strings.ToLower(strings.TrimSpace(r.input.Value()))
		return true, cmd
	}
	if msg.String() == "/" {
		r.filtering = true
		r.input = textinput.New()
		r.input.Prompt = "/"
		r.input.SetValue(r.filter)
		return true, r.input.Focus()
	}
	return false, nil
}

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
	k, ok := msg.(tea.KeyPressMsg)
	if ok {
		if used, cmd := r.filterKey(k); used {
			return r, cmd
		}
	}
	runs := r.visible(ctx)
	r.sync(runs)
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
	if cmd := dispatchForRun(ctx, k.String(), run.ID); cmd != nil {
		return r, cmd
	}
	if cmd := commitKey(ctx, k.String(), run.ID); cmd != nil {
		return r, cmd
	}
	switch k.String() {
	case "enter":
		ctx.Store.SetFocusRun(run.ID)
		ctx.Refresh(fmt.Sprintf("jobs:%d", run.ID))
		return r, push(NewJobs(run))
	case "w":
		ctx.Store.ToggleWatch(run.ID)
	case "o", "O":
		if run.HTMLURL == "" {
			return r, fail(errors.New("no page for this run"))
		}
		return r, linkKey(ctx, k.String(), run.HTMLURL)
	}
	return r, nil
}

func (r *runsScreen) View(ctx *Context, width, height int) string {
	runs := r.visible(ctx)
	var head []string
	if r.filtering {
		head = []string{" " + r.input.View()}
	}
	if len(runs) == 0 {
		msg := "  no runs"
		if r.filter != "" {
			msg = "  no runs match " + r.filter
		}
		return fit(append(head, "", msg), width, height)
	}
	height -= len(head)
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
		// Short, key columns first, so a narrow terminal cuts event and
		// actor rather than the run number, commit or duration.
		rows = append(rows, []string{
			glyph(run.Status, run.Conclusion) + " " + state(run.Status, run.Conclusion), watch,
			fmt.Sprintf("#%d", run.Number), hyperlink(run.CommitURL(), run.ShortSHA()),
			fmtDur(span(run.CreatedAt, end, now)), run.WorkflowName, run.Branch, run.Event, run.Actor,
		})
	}
	return fit(append(head, table([]string{"STATUS", "W", "RUN", "COMMIT", "DURATION", "WORKFLOW", "BRANCH", "EVENT", "ACTOR"}, rows, r.cur.pos-start)...), width, height+len(head))
}

// Resource is what R re-polls.
func (r *runsScreen) Resource() string { return "runs:" + r.repoKey }
