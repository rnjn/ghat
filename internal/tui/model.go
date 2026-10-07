// Package tui is the Bubble Tea front end. Screens read the store; the
// poller's messages tell them when to re-render.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghtui/internal/poller"
	"github.com/rnjn/ghtui/internal/store"
)

// Context is what screens may use: the store, poll refresh, and a clock.
type Context struct {
	Store          *store.Store
	Refresh        func(resource string)
	Now            func() time.Time
	ShowTimestamps bool // initial timestamp toggle in Tail (ui.show_timestamps)
	Actions        Actions
	Open           func(url string) error
}

// Screen is one level of the screen stack.
type Screen interface {
	Update(msg tea.Msg, ctx *Context) (Screen, tea.Cmd)
	View(ctx *Context, width, height int) string
	Title() string
}

// popper is implemented by screens that clean up when Esc leaves them.
type popper interface{ OnPop(ctx *Context) }

// refresher is implemented by screens whose data R can re-poll.
type refresher interface{ Resource() string }

// tickMsg re-renders once a second so ages, elapsed times and the spinner move.
type tickMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

// flashFor is how long a watched run's result stays in the status bar.
const flashFor = 5 * time.Second

// Push asks the model to open a screen on top of the stack.
type Push struct{ Screen Screen }

func push(s Screen) tea.Cmd { return func() tea.Msg { return Push{Screen: s} } }

// pollerMsg wraps a message read from the poller channel.
type pollerMsg struct{ msg any }

// Model is the root Bubble Tea model.
type Model struct {
	ctx     *Context
	msgs    <-chan any
	stack   []Screen
	width   int
	height  int
	help    bool
	status  *statusBar
	authErr error
	confirm *Confirm
}

// NewModel starts on the Board and listens to msgs from the poller.
func NewModel(ctx Context, msgs <-chan any) Model {
	return Model{ctx: &ctx, msgs: msgs, stack: []Screen{NewBoard()}, status: &statusBar{}}
}

// NewModelAt starts on the Runs screen for repoKey, with the Board below it.
func NewModelAt(ctx Context, msgs <-chan any, repoKey string) Model {
	m := NewModel(ctx, msgs)
	m.stack = append(m.stack, NewRuns(repoKey))
	return m
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
func (m Model) Init() tea.Cmd { return tea.Batch(waitFor(m.msgs), tick()) }

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
	case Confirm:
		m.confirm = &msg
		m.status.prompt = msg.Prompt + " [y/N]"
		return m, nil
	case ActionResult:
		now := m.ctx.Now()
		if msg.Err != nil {
			m.status.observe(poller.PollerError{Resource: "action", Err: msg.Err}, now)
			return m, m.forward(msg)
		}
		m.status.flash(msg.Text, now.Add(flashFor))
		if msg.From != nil && len(m.stack) > 1 && m.top() == msg.From {
			m.pop() // the form that sent it
			if _, ok := m.top().(*dispatchPicker); ok && len(m.stack) > 1 {
				m.pop()
			}
		}
		return m, m.forward(msg)
	case tickMsg:
		return m, tick()
	case pollerMsg:
		now := m.ctx.Now()
		m.status.observe(msg.msg, now)
		var extra tea.Cmd
		switch pm := msg.msg.(type) {
		case poller.AuthFailed:
			m.authErr = pm.Err
			m.confirm, m.status.prompt = nil, "" // never send after the token is rejected
		case poller.RunCompleted:
			r := pm.Run
			m.status.flash(fmt.Sprintf("%s #%d %s: %s", r.RepoKey, r.Number, r.WorkflowName, state(r.Status, r.Conclusion)), now.Add(flashFor))
			extra = tea.Raw("\a")
		}
		cmd := m.forward(msg.msg)
		return m, tea.Batch(cmd, extra, waitFor(m.msgs))
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, m.forward(msg)
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}
	if m.confirm != nil {
		c := m.confirm
		m.confirm, m.status.prompt = nil, ""
		if k == "y" {
			if c.Accepted != nil {
				c.Accepted()
			}
			return m, c.Run
		}
		m.status.flash("cancelled", m.ctx.Now().Add(flashFor))
		return m, nil
	}
	if m.authErr == nil && !m.help {
		if c, ok := m.top().(keyCapturer); ok && c.CapturesKeys() && k != "esc" {
			return m, m.forward(msg)
		}
	}
	if k == "q" {
		return m, tea.Quit
	}
	if m.authErr != nil {
		return m, nil
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
	case "R":
		if r, ok := m.top().(refresher); ok {
			m.ctx.Refresh(r.Resource())
		}
		return m, nil
	case "esc":
		if h, ok := m.top().(escHandler); ok && h.HandleEsc() {
			return m, nil
		}
		if len(m.stack) > 1 {
			m.pop()
		}
		return m, nil
	}
	return m, m.forward(msg)
}

// pop closes the top screen.
func (m *Model) pop() {
	if p, ok := m.top().(popper); ok {
		p.OnPop(m.ctx)
	}
	m.stack = m.stack[:len(m.stack)-1]
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
	if m.authErr != nil {
		body = fit([]string{"", "  GitHub rejected the token. Run `gh auth login`, then restart ghtui.", "", "  " + m.authErr.Error(), "", "  q quit"}, m.width, bodyH)
	} else if m.help {
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
