package tui

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghtui/internal/actions"
	"github.com/rnjn/ghtui/internal/gh"
	"github.com/rnjn/ghtui/internal/store"
	"github.com/rnjn/ghtui/internal/workflow"
)

// Actions performs operations on GitHub; *actions.Service satisfies it.
type Actions interface {
	Rerun(ctx context.Context, run gh.Run) (string, error)
	Cancel(ctx context.Context, run gh.Run) (string, error)
	Dispatchable(ctx context.Context, owner, repo, ref string) ([]actions.Dispatchable, error)
	Dispatch(ctx context.Context, owner, repo string, wf gh.Workflow, ref string, inputs []workflow.Input, values map[string]string) (string, error)
}

func fail(err error) tea.Cmd { return func() tea.Msg { return ActionResult{Err: err} } }

// runKey handles r (rerun) and x (cancel) for the run with runID, using
// its latest state from the store. It returns nil for other keys.
func runKey(ctx *Context, k string, runID int64) tea.Cmd {
	if k != "r" && k != "x" {
		return nil
	}
	run, ok := ctx.Store.Run(runID)
	if !ok {
		return fail(errors.New("run is no longer listed"))
	}
	label := fmt.Sprintf("%s #%d %s", run.RepoKey, run.Number, run.WorkflowName)
	var verb string
	var do func(context.Context, gh.Run) (string, error)
	switch {
	case k == "r" && store.IsActive(run.Status):
		return fail(errors.New("run is still in progress"))
	case k == "x" && run.Status == "completed":
		return fail(errors.New("run already finished"))
	case k == "r":
		verb, do = "Rerun", ctx.Actions.Rerun
	default:
		verb, do = "Cancel", ctx.Actions.Cancel
	}
	refresh, st := ctx.Refresh, ctx.Store
	return ask(fmt.Sprintf("%s %s?", verb, label), func() tea.Msg {
		// The run may have changed between the key press and y.
		if latest, ok := st.Run(run.ID); ok {
			switch {
			case k == "r" && store.IsActive(latest.Status):
				return ActionResult{Err: errors.New("run is still in progress")}
			case k == "x" && latest.Status == "completed":
				return ActionResult{Err: errors.New("run already finished")}
			}
			run = latest
		}
		text, err := do(context.Background(), run)
		if err == nil {
			refresh("runs:" + run.RepoKey)
			refresh(fmt.Sprintf("jobs:%d", run.ID))
		}
		return ActionResult{Text: text, Err: err}
	})
}

// openURL opens url in the browser and reports the outcome.
func openURL(ctx *Context, url string) tea.Cmd {
	open := ctx.Open
	return func() tea.Msg {
		if err := open(url); err != nil {
			return ActionResult{Err: fmt.Errorf("open %s: %w", url, err)}
		}
		return ActionResult{Text: "opened in browser"}
	}
}

// runURL is the run's page, from the store when it has one.
func runURL(ctx *Context, runID int64) string {
	if r, ok := ctx.Store.Run(runID); ok && r.HTMLURL != "" {
		return r.HTMLURL
	}
	return ""
}
