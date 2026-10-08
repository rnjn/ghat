package tui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/store"
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

// visible is the repo's runs that match the filter.
func (r *runsScreen) visible(ctx *Context) []gh.Run {
	runs := ctx.Store.Runs(r.repoKey)
	if r.filter == "" {
		return runs
	}
	var out []gh.Run
	for _, run := range runs {
		if matchRun(run, r.filter) {
			out = append(out, run)
		}
	}
	return out
}

// statusAliases name groups of states a filter term can select.
var statusAliases = map[string]func(gh.Run) bool{
	"failed": func(r gh.Run) bool { return r.Status == "completed" && isFailure(r.Conclusion) },
	"active": func(r gh.Run) bool { return store.IsActive(r.Status) },
	"done":   func(r gh.Run) bool { return r.Status == "completed" },
}

// statusPresets is what s cycles through.
var statusPresets = []string{"failed", "active", "queued", "done", ""}

// matchRun reports whether run matches every space-separated term of
// filter. A term is a status alias or a substring of the branch, status or
// conclusion; a leading - negates it.
func matchRun(run gh.Run, filter string) bool {
	for _, term := range strings.Fields(filter) {
		negate := strings.HasPrefix(term, "-")
		term = strings.TrimPrefix(term, "-")
		if term == "" {
			continue
		}
		var hit bool
		if alias, ok := statusAliases[term]; ok {
			hit = alias(run)
		} else {
			hit = strings.Contains(strings.ToLower(run.Branch), term) ||
				strings.Contains(strings.ToLower(state(run.Status, run.Conclusion)), term) ||
				strings.Contains(strings.ToLower(run.Status), term)
		}
		if hit == negate {
			return false
		}
	}
	return true
}

// cycleStatus moves the filter to the next status preset.
func (r *runsScreen) cycleStatus() {
	next := 0
	for i, p := range statusPresets {
		if p == r.filter {
			next = (i + 1) % len(statusPresets)
		}
	}
	r.filter = statusPresets[next]
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
	if msg.String() == "s" {
		r.cycleStatus()
		return true, nil
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
	if cmd := pipelineViewKey(ctx, k.String(), run.ID); cmd != nil {
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
