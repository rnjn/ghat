package tui

import tea "charm.land/bubbletea/v2"

// Confirm asks for a single-key y/N in the status bar before running Run.
type Confirm struct {
	Prompt string
	Run    tea.Cmd
}

// ActionResult reports the outcome of an action: Text on success, Err on
// failure.
type ActionResult struct {
	Text string
	Err  error
	From Screen // on success, close this screen if it is still on top
}

// keyCapturer is implemented by screens that take typed text; the model
// then passes them every key except ctrl+c and esc.
type keyCapturer interface{ CapturesKeys() bool }

// ask returns a command that opens a confirmation prompt.
func ask(prompt string, run tea.Cmd) tea.Cmd {
	return func() tea.Msg { return Confirm{Prompt: prompt, Run: run} }
}
