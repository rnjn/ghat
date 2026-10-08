package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghat/internal/actions"
	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/poller"
)

func TestDispatchFormLockedWhileInFlight(t *testing.T) {
	ctx, _, fa := actCtx(t)
	m := NewModel(*ctx, make(chan any))
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 14})
	m, _ = update(m, Push{Screen: newDispatchForm("acme/api", "main", formFixture())})
	for _, k := range []string{"tab", "tab", "tab"} {
		m, _ = update(m, key(k))
	}
	m, _ = update(m, key("v"))
	m, _ = update(m, key("tab")) // n, last field
	m, cmd := update(m, key("enter"))
	m, _ = update(m, cmd()) // Confirm
	m, run := update(m, key("y"))
	// while the request is in flight: more keys do nothing
	m, again := update(m, key("enter"))
	if again != nil {
		if _, isConfirm := again().(Confirm); isConfirm {
			t.Fatal("second submit while in flight")
		}
	}
	m, _ = update(m, key("y"))
	if v := plain(m.View().Content); strings.Contains(v, "3y") || !strings.Contains(v, "dispatching") {
		t.Fatalf("form not locked:\n%s", v)
	}
	fa.err = errors.New("boom")
	m, _ = update(m, run()) // the dispatch fails
	if fa.calls[0] != "dispatch 1 main" || len(fa.calls) != 1 {
		t.Fatalf("calls %v", fa.calls)
	}
	_, cmd = update(m, key("enter")) // unlocked again
	if cmd == nil {
		t.Fatal("form still locked after an error")
	}
	if _, ok := cmd().(Confirm); !ok {
		t.Fatal("enter after error did not ask again")
	}
}

func TestPickerShowsUnparseableWorkflows(t *testing.T) {
	ctx, _, fa := actCtx(t)
	fa.dispatchable = append(sampleDispatchable(), actions.Dispatchable{
		Workflow: gh.Workflow{ID: 9, Name: "Broken", Path: ".github/workflows/broken.yml"},
		Err:      errors.New("yaml: line 3: did not find expected key"),
	})
	p := openPicker(t, ctx, NewBoard())
	v := plain(p.View(ctx, 120, 8))
	if !strings.Contains(v, "Broken") || !strings.Contains(v, "error: yaml: line 3") {
		t.Fatalf("view:\n%s", v)
	}
	p, _ = press(p, ctx, "j", "j")
	if _, cmd := p.Update(key("enter"), ctx); cmd != nil {
		if _, ok := cmd().(Push); ok {
			t.Fatal("opened a form for an unparseable workflow")
		}
	}
}

func TestAuthFailedClosesPrompt(t *testing.T) {
	m, f := confirmModel(t)
	m, _ = sendPoller(m, poller.AuthFailed{Err: errors.New("401")})
	_, cmd := update(m, key("y"))
	if cmd != nil {
		for _, msg := range safeRun(cmd) {
			if _, ok := msg.(ActionResult); ok {
				t.Fatal("action ran after auth failed")
			}
		}
	}
	if f.n != 0 {
		t.Fatal("action ran")
	}
}

func TestYesRechecksRunStatus(t *testing.T) {
	ctx, _, fa := actCtx(t)
	s := NewRuns("acme/api")
	s, _ = press(s, ctx, "j") // run 11, completed: r allowed
	_, cmd := s.Update(key("r"), ctx)
	c := cmd().(Confirm)
	runs := ctx.Store.Runs("acme/api")
	runs[1].Status, runs[1].Conclusion = "in_progress", "" // rerun started elsewhere
	ctx.Store.SetRuns("acme/api", runs, "")
	res := c.Run().(ActionResult)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "still in progress") || len(fa.calls) != 0 {
		t.Fatalf("res %+v calls %v", res, fa.calls)
	}
}

func TestOpenRunWithoutURL(t *testing.T) {
	ctx, rec, _ := actCtx(t)
	s := NewRuns("acme/api")
	s, _ = press(s, ctx, "j") // run 11 has no HTMLURL in the seed
	_, cmd := s.Update(key("O"), ctx)
	if res := cmd().(ActionResult); res.Err == nil || !strings.Contains(res.Err.Error(), "no page for this run") {
		t.Fatalf("res %+v", res)
	}
	if len(rec.opened) != 0 {
		t.Fatalf("opened %v", rec.opened)
	}
}
