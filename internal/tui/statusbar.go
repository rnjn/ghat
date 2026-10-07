package tui

import (
	"fmt"
	"strings"
	"time"

	"ghtui/internal/poller"
)

// errorTTL is how long a poll error stays in the status bar.
const errorTTL = time.Minute

// statusBar accumulates what the bottom line shows.
type statusBar struct {
	rate     poller.RateLimit
	haveRate bool
	lastPoll time.Time
	lastErr  string
	errAt    time.Time
	flashMsg string
	flashTo  time.Time
	prompt   string
}

// flash shows msg in place of the usual parts until until.
func (s *statusBar) flash(msg string, until time.Time) { s.flashMsg, s.flashTo = msg, until }

// observe updates the bar from a poller message received at at.
func (s *statusBar) observe(msg any, at time.Time) {
	switch m := msg.(type) {
	case poller.RateLimit:
		s.rate, s.haveRate = m, true
	case poller.PollerError:
		s.lastErr, s.errAt = fmt.Sprintf("%s: %v", m.Resource, m.Err), at
	case poller.Polled, poller.ReposUpdated, poller.RunsUpdated, poller.JobsUpdated, poller.LogAppended, poller.LogComplete:
		s.lastPoll = at
	}
}

func (s *statusBar) view(title string, width int, now time.Time) string {
	parts := []string{title}
	if s.prompt != "" {
		parts = append(parts, s.prompt)
	}
	if s.flashMsg != "" && now.Before(s.flashTo) {
		parts = append(parts, "★ "+s.flashMsg)
	}
	switch {
	case s.haveRate && s.rate.Remaining == 0 && s.rate.Reset.After(now):
		parts = append(parts, "rate limited until "+s.rate.Reset.Local().Format("15:04"))
	case s.haveRate:
		parts = append(parts, fmt.Sprintf("quota %d", s.rate.Remaining))
	}
	if !s.lastPoll.IsZero() {
		parts = append(parts, "polled "+fmtAge(now.Sub(s.lastPoll))+" ago")
	}
	if s.lastErr != "" && now.Sub(s.errAt) < errorTTL {
		parts = append(parts, "error "+s.lastErr)
	}
	line := truncate(" "+strings.Join(parts, " │ "), width)
	if width <= 0 {
		return ""
	}
	pad := width - len([]rune(stripForWidth(line)))
	if pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return styleStatus.Render(line)
}
