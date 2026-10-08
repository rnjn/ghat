package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rnjn/ghat/internal/actions"
	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/store"
	"github.com/rnjn/ghat/internal/workflow"
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
			Status: "in_progress", CreatedAt: ago(3 * time.Minute), UpdatedAt: ago(time.Minute), HTMLURL: "https://github.com/acme/api/actions/runs/12"},
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
	opened    []string
	openErr   error
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
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
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

// seedJobs gives run 12 two jobs: build (running, step 3 in progress)
// and lint (done).
func seedJobs(st *store.Store) {
	st.SetJobs(12, []gh.Job{
		{ID: 120, RunID: 12, Name: "build", Status: "in_progress", StartedAt: ago(3 * time.Minute), HTMLURL: "https://github.com/acme/api/actions/runs/12/job/120", Steps: []gh.Step{
			{Number: 1, Name: "Set up job", Status: "completed", Conclusion: "success", StartedAt: ago(3 * time.Minute), CompletedAt: ago(170 * time.Second)},
			{Number: 2, Name: "Checkout", Status: "completed", Conclusion: "success", StartedAt: ago(170 * time.Second), CompletedAt: ago(160 * time.Second)},
			{Number: 3, Name: "Run make test", Status: "in_progress", StartedAt: ago(160 * time.Second)},
			{Number: 4, Name: "Complete job", Status: "pending"},
		}},
		{ID: 121, RunID: 12, Name: "lint", Status: "completed", Conclusion: "failure", StartedAt: ago(3 * time.Minute), CompletedAt: ago(2 * time.Minute), Steps: []gh.Step{
			{Number: 1, Name: "Set up job", Status: "completed", Conclusion: "success", StartedAt: ago(3 * time.Minute), CompletedAt: ago(170 * time.Second)},
			{Number: 2, Name: "Run golangci-lint", Status: "completed", Conclusion: "failure", StartedAt: ago(170 * time.Second), CompletedAt: ago(2 * time.Minute)},
		}},
	})
}

func run12(st *store.Store) gh.Run {
	r, _ := st.Run(12)
	return r
}

// fakeActions records calls and returns scripted results.
type fakeActions struct {
	calls        []string
	err          error
	dispatchable []actions.Dispatchable
	loadErr      error
	dispatched   map[string]string
	dispatchRef  string
}

func (f *fakeActions) Rerun(_ context.Context, run gh.Run) (string, error) {
	f.calls = append(f.calls, fmt.Sprintf("rerun %d", run.ID))
	return "rerun requested", f.err
}

func (f *fakeActions) Cancel(_ context.Context, run gh.Run) (string, error) {
	f.calls = append(f.calls, fmt.Sprintf("cancel %d", run.ID))
	return "cancel requested", f.err
}

func (f *fakeActions) Dispatchable(_ context.Context, owner, repo, ref string) ([]actions.Dispatchable, error) {
	f.calls = append(f.calls, fmt.Sprintf("dispatchable %s/%s@%s", owner, repo, ref))
	return f.dispatchable, f.loadErr
}

func (f *fakeActions) Dispatch(_ context.Context, owner, repo string, wf gh.Workflow, ref string, inputs []workflow.Input, values map[string]string) (string, error) {
	f.calls = append(f.calls, fmt.Sprintf("dispatch %d %s", wf.ID, ref))
	f.dispatched, f.dispatchRef = values, ref
	return "dispatch requested", f.err
}

// actCtx is a context with fake actions and a recording opener.
func actCtx(t *testing.T) (*Context, *ctxRecorder, *fakeActions) {
	t.Helper()
	ctx, rec := testContext(seedStore())
	seedJobs(ctx.Store)
	fa := &fakeActions{}
	ctx.Actions = fa
	ctx.Open = func(u string) error { rec.opened = append(rec.opened, u); return rec.openErr }
	return ctx, rec, fa
}

// confirmed runs the Confirm a screen command produced, as if y was pressed.
func confirmed(t *testing.T, cmd tea.Cmd) ActionResult {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command")
	}
	c, ok := cmd().(Confirm)
	if !ok {
		t.Fatal("command did not ask for confirmation")
	}
	r, ok := c.Run().(ActionResult)
	if !ok {
		t.Fatal("confirmed command did not return an ActionResult")
	}
	return r
}

var errAny = fmt.Errorf("boom")
