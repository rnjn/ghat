package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"ghtui/internal/gh"
)

// jobsScreen shows a run's jobs and the selected job's steps.
type jobsScreen struct {
	run gh.Run
}

// NewJobs returns the jobs screen for run.
func NewJobs(run gh.Run) Screen { return &jobsScreen{run: run} }

func (j *jobsScreen) Title() string {
	return fmt.Sprintf("%s #%d %s", j.run.RepoKey, j.run.Number, j.run.WorkflowName)
}

func (j *jobsScreen) Update(msg tea.Msg, ctx *Context) (Screen, tea.Cmd) { return j, nil }

func (j *jobsScreen) View(ctx *Context, width, height int) string { return fit(nil, width, height) }
