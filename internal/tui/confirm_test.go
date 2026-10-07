package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type fired struct{ n int }

func (f *fired) cmd() tea.Cmd {
	return func() tea.Msg { f.n++; return ActionResult{Text: "rerun of #41 requested"} }
}

func confirmModel(t *testing.T) (Model, *fired) {
	t.Helper()
	m, _ := newTestModel(t)
	f := &fired{}
	m, _ = update(m, Confirm{Prompt: "Rerun acme/api #41 CI?", Run: f.cmd()})
	return m, f
}

func TestConfirmShowsPrompt(t *testing.T) {
	m, _ := confirmModel(t)
	if v := plain(m.View().Content); !strings.Contains(v, "Rerun acme/api #41 CI? [y/N]") {
		t.Fatalf("view:\n%s", v)
	}
}

func TestConfirmYesRunsOnce(t *testing.T) {
	m, f := confirmModel(t)
	m, cmd := update(m, key("y"))
	if cmd == nil {
		t.Fatal("y returned no command")
	}
	msg := cmd()
	m, cmd2 := update(m, key("y"))
	if cmd2 != nil {
		cmd2()
	}
	if f.n != 1 {
		t.Fatalf("action ran %d times", f.n)
	}
	if strings.Contains(plain(m.View().Content), "[y/N]") {
		t.Fatal("prompt still shown")
	}
	m, _ = update(m, msg)
	if v := plain(m.View().Content); !strings.Contains(v, "rerun of #41 requested") {
		t.Fatalf("result not flashed:\n%s", v)
	}
}

func TestConfirmOtherKeysCancel(t *testing.T) {
	for _, k := range []string{"n", "esc", "x", "q", "enter"} {
		m, f := confirmModel(t)
		m, cmd := update(m, key(k))
		if cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Fatalf("%s quit instead of cancelling the prompt", k)
			}
		}
		if f.n != 0 {
			t.Fatalf("%s ran the action", k)
		}
		v := plain(m.View().Content)
		if strings.Contains(v, "[y/N]") || !strings.Contains(v, "cancelled") {
			t.Fatalf("%s: view:\n%s", k, v)
		}
		if m.top().Title() != "Board" {
			t.Fatalf("%s reached the screen", k)
		}
	}
}

func TestActionResultError(t *testing.T) {
	m, _ := newTestModel(t)
	m, _ = update(m, ActionResult{Err: errors.New("GitHub refused (403 Forbidden)")})
	if v := plain(m.View().Content); !strings.Contains(v, "403 Forbidden") {
		t.Fatalf("view:\n%s", v)
	}
}

func TestCtrlCQuitsDuringPrompt(t *testing.T) {
	m, f := confirmModel(t)
	_, cmd := update(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok || f.n != 0 {
		t.Fatal("ctrl+c did not quit")
	}
}

func TestAskBuildsConfirm(t *testing.T) {
	f := &fired{}
	c, ok := ask("Sure?", f.cmd())().(Confirm)
	if !ok || c.Prompt != "Sure?" || c.Run == nil {
		t.Fatalf("got %#v", c)
	}
}
