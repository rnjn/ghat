package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghat/internal/poller"
)

func newTestModel(t *testing.T) (Model, chan any) {
	t.Helper()
	ctx, _ := testContext(seedStore())
	ch := make(chan any, 8)
	m := NewModel(*ctx, ch)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	return mm.(Model), ch
}

func update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	mm, cmd := m.Update(msg)
	return mm.(Model), cmd
}

func TestModelStartsOnBoardAndRendersStatusBar(t *testing.T) {
	m, _ := newTestModel(t)
	v := plain(m.View().Content)
	if !strings.Contains(v, "acme/api") || !strings.Contains(v, "ghat ▸ Repositories") {
		t.Fatalf("view:\n%s", v)
	}
	if !m.View().AltScreen {
		t.Fatal("not using the alt screen")
	}
}

func TestModelPushAndEscPop(t *testing.T) {
	m, _ := newTestModel(t)
	m, cmd := update(m, key("enter"))
	m, _ = update(m, cmd())
	if !strings.Contains(plain(m.View().Content), "acme/api") || m.top().Title() != "acme/api" {
		t.Fatalf("after enter top = %q", m.top().Title())
	}
	m, _ = update(m, key("esc"))
	if m.top().Title() != "Board" {
		t.Fatalf("after esc top = %q", m.top().Title())
	}
	m, _ = update(m, key("esc"))
	if m.top().Title() != "Board" {
		t.Fatal("esc popped the last screen")
	}
}

func TestModelQuitAndHelp(t *testing.T) {
	m, _ := newTestModel(t)
	m, _ = update(m, key("?"))
	if !strings.Contains(plain(m.View().Content), "Keys") {
		t.Fatal("help overlay not shown")
	}
	m, _ = update(m, key("esc"))
	if strings.Contains(plain(m.View().Content), "Keys") {
		t.Fatal("esc did not close help")
	}
	_, cmd := update(m, key("q"))
	if cmd == nil {
		t.Fatal("q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q did not quit")
	}
}

func TestModelReadsPollerChannel(t *testing.T) {
	m, ch := newTestModel(t)
	ch <- poller.RateLimit{Remaining: 77}
	if m.Init() == nil {
		t.Fatal("Init returned no command")
	}
	msg := waitFor(ch)()
	m, cmd := update(m, msg)
	if !strings.Contains(plain(m.View().Content), "77") {
		t.Fatal("RateLimit not reflected in status bar")
	}
	if cmd == nil {
		t.Fatal("channel read not re-armed")
	}
	close(ch)
	if got := cmd(); got != nil {
		t.Fatalf("closed channel produced %#v", got)
	}
}

func TestModelTinyWindow(t *testing.T) {
	m, _ := newTestModel(t)
	m, _ = update(m, tea.WindowSizeMsg{Width: 40, Height: 10})
	assertFits(t, m.View().Content, 40)
	m, _ = update(m, tea.WindowSizeMsg{Width: 0, Height: 0})
	_ = m.View()
}

func TestNewModelAtOpensOnRuns(t *testing.T) {
	ctx, _ := testContext(seedStore())
	m := NewModelAt(*ctx, make(chan any), "acme/api")
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 10})
	if m.top().Title() != "acme/api" || !strings.Contains(plain(m.View().Content), "feat/login") {
		t.Fatalf("top %q", m.top().Title())
	}
	m, _ = update(m, key("esc"))
	if m.top().Title() != "Board" {
		t.Fatal("esc did not return to Board")
	}
}

func TestModelPassesVersionToFooter(t *testing.T) {
	ctx, _ := testContext(seedStore())
	ctx.Version = "ghat v9.9.9"
	m := NewModel(*ctx, make(chan any))
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 20})
	lines := strings.Split(plain(m.View().Content), "\n")
	if !strings.Contains(lines[len(lines)-1], "ghat v9.9.9") {
		t.Fatalf("footer %q", lines[len(lines)-1])
	}
}
