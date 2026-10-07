package tui

import (
	tea "charm.land/bubbletea/v2"

	"ghtui/internal/gh"
)

// tailScreen shows one job: live steps while it runs, its log once done.
type tailScreen struct {
	owner, repo string
	job         gh.Job
	step        int // step to scroll to on first render; 0 = top
}

// NewTail opens job; step, when non-zero, is the step to scroll to.
func NewTail(owner, repo string, job gh.Job, step int) Screen {
	return &tailScreen{owner: owner, repo: repo, job: job, step: step}
}

func (t *tailScreen) Title() string { return t.job.Name }

func (t *tailScreen) Update(msg tea.Msg, ctx *Context) (Screen, tea.Cmd) { return t, nil }

func (t *tailScreen) View(ctx *Context, width, height int) string { return fit(nil, width, height) }
