package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"ghtui/internal/actions"
	"ghtui/internal/gh"
	"ghtui/internal/workflow"
)

func formFixture() actions.Dispatchable {
	return actions.Dispatchable{
		Workflow: gh.Workflow{ID: 1, Name: "Deploy"},
		Inputs: []workflow.Input{
			{Name: "env", Type: "choice", Options: []string{"staging", "prod"}, Default: "staging", Required: true, Description: "Where to deploy"},
			{Name: "dry", Type: "boolean", Default: "true"},
			{Name: "tag", Type: "string", Required: true, Description: "Image tag"},
			{Name: "n", Type: "number", Default: "3"},
		},
	}
}

func newForm(t *testing.T) (*Context, *ctxRecorder, *fakeActions, Screen) {
	t.Helper()
	ctx, rec, fa := actCtx(t)
	return ctx, rec, fa, newDispatchForm("acme/api", "main", formFixture())
}

func press(s Screen, ctx *Context, keys ...string) (Screen, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		s, cmd = s.Update(key(k), ctx)
	}
	return s, cmd
}

func typeText(s Screen, ctx *Context, text string) Screen {
	for _, r := range text {
		s, _ = s.Update(tea.KeyPressMsg{Code: r, Text: string(r)}, ctx)
	}
	return s
}

func TestDispatchFormGolden(t *testing.T) {
	ctx, _, _, f := newForm(t)
	golden.RequireEqual(t, plain(f.View(ctx, 80, 12)))
}

func TestDispatchFormEditing(t *testing.T) {
	ctx, _, fa, f := newForm(t)
	f, _ = press(f, ctx, "tab")           // env
	f, _ = press(f, ctx, "space")         // staging → prod
	f, _ = press(f, ctx, "down", "space") // dry true → false
	f, _ = press(f, ctx, "tab")           // tag
	f = typeText(f, ctx, "v2.1")
	f, _ = press(f, ctx, "shift+tab", "shift+tab", "right") // env prod → staging
	f, _ = press(f, ctx, "tab", "tab", "tab")               // n (last)
	_, cmd := press(f, ctx, "enter")
	res := confirmed(t, cmd)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	want := map[string]string{"env": "staging", "dry": "false", "tag": "v2.1", "n": "3"}
	for k, v := range want {
		if fa.dispatched[k] != v {
			t.Fatalf("values %v, want %v", fa.dispatched, want)
		}
	}
	if fa.dispatchRef != "main" || len(fa.calls) != 1 {
		t.Fatalf("ref %q calls %v", fa.dispatchRef, fa.calls)
	}
}

func TestDispatchFormEnterAdvancesThenValidates(t *testing.T) {
	ctx, _, fa, f := newForm(t)
	var cmd tea.Cmd
	for i := 0; i < 4; i++ { // ref → env → dry → tag → n
		f, cmd = press(f, ctx, "enter")
		if cmd != nil {
			if _, isConfirm := cmd().(Confirm); isConfirm {
				t.Fatalf("submitted after %d enters", i+1)
			}
		}
	}
	f, cmd = press(f, ctx, "enter") // submit with tag empty
	if cmd != nil {
		if _, isConfirm := cmd().(Confirm); isConfirm {
			t.Fatal("submitted with a required input empty")
		}
	}
	if v := plain(f.View(ctx, 80, 12)); !strings.Contains(v, `"tag" is required`) {
		t.Fatalf("no validation message:\n%s", v)
	}
	if len(fa.calls) != 0 {
		t.Fatalf("calls %v", fa.calls)
	}
}

func TestDispatchFormChoiceDefaultNotInOptions(t *testing.T) {
	ctx, _, _ := actCtx(t)
	d := formFixture()
	d.Inputs = []workflow.Input{{Name: "env", Type: "choice", Options: []string{"a", "b"}, Default: "zzz", Required: true}}
	f := Screen(newDispatchForm("acme/api", "main", d))
	f, _ = press(f, ctx, "tab")
	f, _ = press(f, ctx, "enter")
	if v := plain(f.View(ctx, 80, 8)); !strings.Contains(v, `"env" must be one of a, b`) {
		t.Fatalf("view:\n%s", v)
	}
}

func TestDispatchFormSuccessPopsAndRefreshes(t *testing.T) {
	ctx, rec, _, f := newForm(t)
	f, _ = press(f, ctx, "tab", "tab", "tab")
	f = typeText(f, ctx, "v1")
	_, cmd := press(f, ctx, "enter", "enter")
	c := cmd().(Confirm)
	if c.Prompt != "Dispatch Deploy on main?" {
		t.Fatalf("prompt %q", c.Prompt)
	}
	res := c.Run().(ActionResult)
	if res.Err != nil || res.Pop != 2 || res.Text != "dispatch requested" {
		t.Fatalf("res %+v", res)
	}
	if len(rec.refreshed) != 1 || rec.refreshed[0] != "runs:acme/api" {
		t.Fatalf("refreshed %v", rec.refreshed)
	}
}

func TestModelPopsOnActionResultAndFormCapturesKeys(t *testing.T) {
	ctx, _, _ := actCtx(t)
	m := NewModel(*ctx, make(chan any))
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 12})
	m, _ = update(m, Push{Screen: NewRuns("acme/api")})
	m, _ = update(m, Push{Screen: &dispatchPicker{repoKey: "acme/api"}})
	m, _ = update(m, Push{Screen: newDispatchForm("acme/api", "main", formFixture())})
	m, _ = update(m, key("tab"))
	m, _ = update(m, key("tab"))
	m, _ = update(m, key("tab"))
	for _, k := range []string{"q", "R", "?", "r", "x", "d", "o"} {
		var cmd tea.Cmd
		m, cmd = update(m, key(k))
		for _, msg := range safeRun(cmd) { // the input's blink command sleeps
			if _, quit := msg.(tea.QuitMsg); quit {
				t.Fatalf("%s quit while typing", k)
			}
		}
	}
	if !strings.Contains(plain(m.View().Content), "qR?rxdo") {
		t.Fatalf("typed keys not in the field:\n%s", plain(m.View().Content))
	}
	m, _ = update(m, ActionResult{Text: "dispatch requested", Pop: 2})
	if m.top().Title() != "acme/api" {
		t.Fatalf("after pop top = %q", m.top().Title())
	}
	m, _ = update(m, ActionResult{Err: errAny, Pop: 2})
	if m.top().Title() != "acme/api" {
		t.Fatal("popped on error")
	}
}

func TestDispatchFormFitsSmallScreens(t *testing.T) {
	ctx, _, _, f := newForm(t)
	assertFits(t, f.View(ctx, 40, 10), 40)
	f, _ = press(f, ctx, "tab", "tab", "tab", "tab")
	if v := plain(f.View(ctx, 40, 5)); !strings.Contains(v, "n") {
		t.Fatalf("focused field scrolled out:\n%s", v)
	}
	_ = f.View(ctx, 0, 0)
}
