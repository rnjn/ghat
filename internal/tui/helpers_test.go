package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"ghtui/internal/gh"
	"ghtui/internal/store"
)

var testNow = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) time.Time { return testNow.Add(-d) }

// seedStore returns a store with three repos: one busy, one red, one
// unavailable.
func seedStore() *store.Store {
	st := store.New()
	st.SetRepos([]store.RepoState{
		{Repo: gh.Repo{Owner: "acme", Name: "api", PushedAt: ago(time.Hour)}},
		{Repo: gh.Repo{Owner: "acme", Name: "web", PushedAt: ago(2 * time.Hour)}},
		{Repo: gh.Repo{Owner: "me", Name: "dots", PushedAt: ago(3 * time.Hour)}},
	})
	st.SetRuns("acme/web", []gh.Run{
		{ID: 21, RepoKey: "acme/web", Number: 8, WorkflowName: "CI", Branch: "main", Event: "push", Actor: "bob",
			Status: "completed", Conclusion: "failure", CreatedAt: ago(30 * time.Minute), UpdatedAt: ago(25 * time.Minute)},
	}, "")
	st.SetRuns("acme/api", []gh.Run{
		{ID: 12, RepoKey: "acme/api", Number: 41, WorkflowName: "CI", Branch: "feat/login", Event: "pull_request", Actor: "alice",
			Status: "in_progress", CreatedAt: ago(3 * time.Minute), UpdatedAt: ago(time.Minute)},
		{ID: 11, RepoKey: "acme/api", Number: 40, WorkflowName: "Deploy", Branch: "main", Event: "push", Actor: "alice",
			Status: "completed", Conclusion: "success", CreatedAt: ago(2 * time.Hour), UpdatedAt: ago(110 * time.Minute)},
		{ID: 10, RepoKey: "acme/api", Number: 39, WorkflowName: "CI", Branch: "main", Event: "push", Actor: "carol",
			Status: "completed", Conclusion: "failure", CreatedAt: ago(3 * time.Hour), UpdatedAt: ago(170 * time.Minute)},
	}, "")
	st.MarkUnavailable("me/dots", "GitHub API: 404 Not Found")
	return st
}

type ctxRecorder struct {
	refreshed []string
}

func testContext(st *store.Store) (*Context, *ctxRecorder) {
	rec := &ctxRecorder{}
	return &Context{
		Store:   st,
		Refresh: func(r string) { rec.refreshed = append(rec.refreshed, r) },
		Now:     func() time.Time { return testNow },
	}, rec
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// plain strips ANSI styling for golden comparison.
func plain(s string) string { return ansi.Strip(s) }

// assertFits fails if any line is wider than width.
func assertFits(t *testing.T, view string, width int) {
	t.Helper()
	for i, l := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(l); w > width {
			t.Fatalf("line %d is %d wide, max %d: %q", i, w, width, plain(l))
		}
	}
}

// pushed extracts the screen from a command that returns a Push message.
func pushed(t *testing.T, cmd tea.Cmd) Screen {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command")
	}
	p, ok := cmd().(Push)
	if !ok {
		t.Fatalf("command did not push a screen")
	}
	return p.Screen
}
