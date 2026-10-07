package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"ghtui/internal/store"
)

// board lists every discovered repo with its latest run.
type board struct {
	cur    cursor
	selKey string
}

// sync re-finds the selected repo by key after the order changed.
func (b *board) sync(repos []store.RepoState) {
	for i, r := range repos {
		if r.Repo.Key() == b.selKey {
			b.cur.pos = i
			return
		}
	}
	b.cur.clamp(len(repos))
	b.remember(repos)
}

func (b *board) remember(repos []store.RepoState) {
	if len(repos) > 0 {
		b.selKey = repos[b.cur.pos].Repo.Key()
	}
}

// NewBoard returns the top-level repo screen.
func NewBoard() Screen { return &board{} }

func (b *board) Title() string { return "Board" }

func (b *board) Update(msg tea.Msg, ctx *Context) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return b, nil
	}
	repos := ctx.Store.Repos()
	b.sync(repos)
	if b.cur.navKey(k.String(), len(repos), 10) {
		b.remember(repos)
		return b, nil
	}
	if k.String() == "enter" && len(repos) > 0 {
		b.cur.clamp(len(repos))
		return b, push(NewRuns(repos[b.cur.pos].Repo.Key()))
	}
	return b, nil
}

func (b *board) View(ctx *Context, width, height int) string {
	repos := ctx.Store.Repos()
	if len(repos) == 0 {
		return fit([]string{"", "  discovering repos…"}, width, height)
	}
	b.sync(repos)
	now := ctx.Now()
	start, end := b.cur.window(len(repos), height-1)
	var rows [][]string
	for _, r := range repos[start:end] {
		rows = append(rows, boardRow(ctx.Store, r, now))
	}
	return fit(table([]string{" ", "REPO", "BRANCH", "WORKFLOW", "AGE", "RUNNING", "FAILED"}, rows, b.cur.pos-start), width, height)
}

func boardRow(st *store.Store, r store.RepoState, now time.Time) []string {
	key := r.Repo.Key()
	if r.Unavailable {
		return []string{styleWarning.Render("!"), styleDim.Render(key), styleDim.Render(r.LastError), "", "", "", ""}
	}
	runs := st.Runs(key)
	if len(runs) == 0 {
		return []string{styleDim.Render("·"), key, styleDim.Render("no runs"), "", "", "", ""}
	}
	running, failed := 0, 0
	for _, run := range runs {
		if store.IsActive(run.Status) {
			running++
		}
		if run.Status == "completed" && isFailure(run.Conclusion) {
			failed++
		}
	}
	latest := runs[0]
	return []string{
		glyph(latest.Status, latest.Conclusion), key, latest.Branch, latest.WorkflowName,
		fmtAge(now.Sub(latest.CreatedAt)), count(running), count(failed),
	}
}

func count(n int) string {
	if n == 0 {
		return styleDim.Render("-")
	}
	return fmt.Sprint(n)
}

// Resource is what R re-polls.
func (b *board) Resource() string { return "repos" }
