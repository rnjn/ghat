// Package tui is the Bubble Tea front end. Screens read the store; the
// poller's messages tell them when to re-render.
package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"ghtui/internal/store"
)

// Context is what screens may use: the store, poll refresh, and a clock.
type Context struct {
	Store   *store.Store
	Refresh func(resource string)
	Now     func() time.Time
}

// Screen is one level of the screen stack.
type Screen interface {
	Update(msg tea.Msg, ctx *Context) (Screen, tea.Cmd)
	View(ctx *Context, width, height int) string
	Title() string
}

// popper is implemented by screens that clean up when Esc leaves them.
type popper interface{ OnPop(ctx *Context) }

// Push asks the model to open a screen on top of the stack.
type Push struct{ Screen Screen }

func push(s Screen) tea.Cmd { return func() tea.Msg { return Push{Screen: s} } }

// pollerMsg wraps a message read from the poller channel.
type pollerMsg struct{ msg any }

// Model is the root Bubble Tea model.
type Model struct {
	ctx    *Context
	msgs   <-chan any
	stack  []Screen
	width  int
	height int
	help   bool
	status *statusBar
}

// NewModel starts on the Board and listens to msgs from the poller.
func NewModel(ctx Context, msgs <-chan any) Model {
	return Model{ctx: &ctx, msgs: msgs, stack: []Screen{NewBoard()}, status: &statusBar{}}
}

func waitFor(ch <-chan any) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return nil
		}
		return pollerMsg{msg: m}
	}
}

// Init starts reading poller messages.
func (m Model) Init() tea.Cmd { return waitFor(m.msgs) }

func (m Model) top() Screen { return m.stack[len(m.stack)-1] }

// Update routes keys, pushes and poller messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case Push:
		m.stack = append(m.stack, msg.Screen)
		return m, nil
	case pollerMsg:
		m.status.observe(msg.msg, m.ctx.Now())
		cmd := m.forward(msg.msg)
		return m, tea.Batch(cmd, waitFor(m.msgs))
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, m.forward(msg)
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" || k == "q" {
		return m, tea.Quit
	}
	if m.help {
		if k == "esc" || k == "?" {
			m.help = false
		}
		return m, nil
	}
	switch k {
	case "?":
		m.help = true
		return m, nil
	case "esc":
		if len(m.stack) > 1 {
			if p, ok := m.top().(popper); ok {
				p.OnPop(m.ctx)
			}
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m, nil
	}
	return m, m.forward(msg)
}

// forward sends msg to the top screen.
func (m Model) forward(msg tea.Msg) tea.Cmd {
	s, cmd := m.top().Update(msg, m.ctx)
	m.stack[len(m.stack)-1] = s
	return cmd
}

// View renders the top screen (or help) above the status bar.
func (m Model) View() tea.View {
	v := tea.View{AltScreen: true}
	if m.width <= 0 || m.height <= 0 {
		return v
	}
	bodyH := m.height - 1
	body := ""
	if m.help {
		body = helpView(m.width, bodyH)
	} else {
		body = fit(strings.Split(m.top().View(m.ctx, m.width, bodyH), "\n"), m.width, bodyH)
	}
	bar := m.status.view(m.top().Title(), m.width, m.ctx.Now())
	if bodyH <= 0 {
		v.Content = bar
	} else {
		v.Content = body + "\n" + bar
	}
	return v
}
