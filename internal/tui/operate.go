package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghat/internal/actions"
	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/store"
	"github.com/rnjn/ghat/internal/workflow"
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

// commitKey opens (c) or copies (C) the commit link of run runID.
func commitKey(ctx *Context, k string, runID int64) tea.Cmd {
	if k != "c" && k != "C" {
		return nil
	}
	run, ok := ctx.Store.Run(runID)
	if !ok || run.CommitURL() == "" {
		return fail(errors.New("no commit for this run"))
	}
	return linkKey(ctx, k, run.CommitURL())
}

// linkKey opens url for a lowercase key (o, c) and copies it for the
// uppercase one (O, C).
func linkKey(ctx *Context, k, url string) tea.Cmd {
	if k == strings.ToUpper(k) {
		return copyURL(url)
	}
	return openURL(ctx, url)
}

// copyURL puts url on the clipboard of the terminal the user sits at (OSC
// 52 works through SSH and, with set-clipboard on, tmux) and shows it, so
// it can still be selected by hand where OSC 52 is not supported.
func copyURL(url string) tea.Cmd {
	return tea.Batch(tea.SetClipboard(url), func() tea.Msg { return ActionResult{Text: "copied " + url} })
}
