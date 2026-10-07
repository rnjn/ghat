package tui

import (
	tea "charm.land/bubbletea/v2"

	"ghtui/internal/actions"
)

// dispatchForm collects the ref and inputs for one workflow.
type dispatchForm struct {
	repoKey string
	ref     string
	wf      actions.Dispatchable
}

func newDispatchForm(repoKey, ref string, wf actions.Dispatchable) *dispatchForm {
	return &dispatchForm{repoKey: repoKey, ref: ref, wf: wf}
}

func (f *dispatchForm) Title() string { return "Dispatch " + f.wf.Workflow.Name }

func (f *dispatchForm) Update(msg tea.Msg, ctx *Context) (Screen, tea.Cmd) { return f, nil }

func (f *dispatchForm) View(ctx *Context, width, height int) string { return fit(nil, width, height) }
