package tui

import tea "charm.land/bubbletea/v2"

// runsScreen lists one repo's runs.
type runsScreen struct {
	repoKey string
}

// NewRuns returns the runs screen for repoKey.
func NewRuns(repoKey string) Screen { return &runsScreen{repoKey: repoKey} }

func (r *runsScreen) Title() string { return r.repoKey }

func (r *runsScreen) Update(msg tea.Msg, ctx *Context) (Screen, tea.Cmd) { return r, nil }

func (r *runsScreen) View(ctx *Context, width, height int) string {
	return fit([]string{r.repoKey}, width, height)
}
