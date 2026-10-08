package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghat/internal/actions"
	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/workflow"
)

func sampleDispatchable() []actions.Dispatchable {
	return []actions.Dispatchable{
		{Workflow: gh.Workflow{ID: 1, Name: "Deploy", Path: ".github/workflows/deploy.yml"},
			Inputs: []workflow.Input{{Name: "env", Type: "choice", Options: []string{"staging", "prod"}, Default: "staging", Required: true}}},
		{Workflow: gh.Workflow{ID: 7, Name: "CI", Path: ".github/workflows/ci.yml"}},
	}
}

// openPicker presses d on s and returns the pushed picker with its load
// result applied.
func openPicker(t *testing.T, ctx *Context, s Screen) Screen {
	t.Helper()
	_, cmd := s.Update(key("d"), ctx)
	if cmd == nil {
		t.Fatal("d returned no command")
	}
	var picker Screen
	var loaded []tea.Msg
	for _, m := range runCmd(cmd) {
		if p, ok := m.(Push); ok {
			picker = p.Screen
		} else {
			loaded = append(loaded, m)
		}
	}
	if picker == nil {
		t.Fatal("d did not push a picker")
	}
	if v := plain(picker.View(ctx, 80, 6)); !strings.Contains(v, "loading workflows") {
		t.Fatalf("no loading state:\n%s", v)
	}
	for _, m := range loaded {
		picker, _ = picker.Update(m, ctx)
	}
	return picker
}

func TestDispatchPickerFromEachScreen(t *testing.T) {
	ctx, _, fa := actCtx(t)
	fa.dispatchable = sampleDispatchable()
	for _, tc := range []struct {
		s       Screen
		wantRef string
	}{
		{NewBoard(), "main"},                // no run: default branch fallback
		{NewRuns("acme/api"), "feat/login"}, // selected run's branch
		{NewJobs(run12(ctx.Store)), "feat/login"},
		{NewTail("acme", "api", ctx.Store.Jobs(12)[0], 0), "feat/login"},
	} {
		fa.calls = nil
		p := openPicker(t, ctx, tc.s)
		if len(fa.calls) != 1 || fa.calls[0] != "dispatchable acme/api@"+tc.wantRef {
			t.Fatalf("%s: calls %v", tc.s.Title(), fa.calls)
		}
		v := plain(p.View(ctx, 100, 6))
		if !strings.Contains(v, "Deploy") || !strings.Contains(v, ".github/workflows/ci.yml") {
			t.Fatalf("%s: view:\n%s", tc.s.Title(), v)
		}
	}
}

func TestDispatchPickerUsesRepoDefaultBranch(t *testing.T) {
	ctx, _, fa := actCtx(t)
	rs := ctx.Store.Repos()
	for i := range rs {
		if rs[i].Repo.Key() == "acme/api" {
			rs[i].Repo.DefaultBranch = "trunk"
		}
	}
	ctx.Store.SetRepos(rs)
	openPicker(t, ctx, NewBoard())
	if fa.calls[0] != "dispatchable acme/api@trunk" {
		t.Fatalf("calls %v", fa.calls)
	}
}

func TestDispatchPickerPreselectsRunWorkflow(t *testing.T) {
	ctx, _, fa := actCtx(t)
	fa.dispatchable = sampleDispatchable()
	runs := ctx.Store.Runs("acme/api")
	runs[0].WorkflowID = 7
	ctx.Store.SetRuns("acme/api", runs, "")
	p := openPicker(t, ctx, NewRuns("acme/api"))
	_, cmd := p.Update(key("enter"), ctx)
	if f := pushed(t, cmd).(*dispatchForm); f.wf.Workflow.ID != 7 || f.ref != "feat/login" {
		t.Fatalf("form for %d on %s", f.wf.Workflow.ID, f.ref)
	}
}

func TestDispatchPickerEmptyAndError(t *testing.T) {
	ctx, _, fa := actCtx(t)
	p := openPicker(t, ctx, NewBoard())
	if v := plain(p.View(ctx, 80, 6)); !strings.Contains(v, "no workflows with a workflow_dispatch trigger") {
		t.Fatalf("view:\n%s", v)
	}
	if _, cmd := p.Update(key("enter"), ctx); cmd != nil {
		t.Fatal("enter on empty picker pushed")
	}
	fa.loadErr = errors.New("GitHub API: 502")
	p = openPicker(t, ctx, NewBoard())
	if v := plain(p.View(ctx, 80, 6)); !strings.Contains(v, "502") {
		t.Fatalf("view:\n%s", v)
	}
	assertFits(t, p.View(ctx, 40, 10), 40)
}
