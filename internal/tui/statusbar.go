package tui

import (
	"fmt"
	"strings"
	"time"

	"ghtui/internal/poller"
)

// maxTitle keeps long screen titles from crowding out the status bar.
const maxTitle = 32

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
	offline  bool
}

// flash shows msg in place of the usual parts until until.
func (s *statusBar) flash(msg string, until time.Time) { s.flashMsg, s.flashTo = msg, until }

// observe updates the bar from a poller message received at at.
func (s *statusBar) observe(msg any, at time.Time) {
	switch m := msg.(type) {
	case poller.RateLimit:
		s.rate, s.haveRate = m, true
	case poller.PollerError:
		if m.Resource == "action" {
			s.lastErr = m.Err.Error()
		} else {
			s.lastErr = fmt.Sprintf("%s: %v", m.Resource, m.Err)
		}
		s.errAt = at
	case poller.RunsFailed:
		s.offline = true
	case poller.Polled:
		s.lastPoll, s.offline = at, false
	case poller.ReposUpdated, poller.RunsUpdated, poller.JobsUpdated, poller.LogAppended, poller.LogComplete:
		s.lastPoll = at
	}
}

func (s *statusBar) view(title string, width int, now time.Time) string {
	parts := []string{truncate(title, maxTitle)}
	if s.prompt != "" {
		parts = append(parts, s.prompt)
	}
	if s.flashMsg != "" && now.Before(s.flashTo) {
		parts = append(parts, "★ "+s.flashMsg)
	}
	if s.lastErr != "" && now.Sub(s.errAt) < errorTTL {
		parts = append(parts, "✗ "+s.lastErr)
	}
	switch {
	case s.haveRate && s.rate.Remaining == 0 && s.rate.Reset.After(now):
		parts = append(parts, "rate limited until "+s.rate.Reset.Local().Format("15:04"))
	case s.haveRate && s.rate.Remaining < 500:
		parts = append(parts, fmt.Sprintf("quota %d low, polling slowed", s.rate.Remaining))
	case s.haveRate:
		parts = append(parts, fmt.Sprintf("quota %d", s.rate.Remaining))
	}
	switch {
	case s.offline && !s.lastPoll.IsZero():
		parts = append(parts, "offline, retrying (data "+fmtAge(now.Sub(s.lastPoll))+" old)")
	case s.offline:
		parts = append(parts, "offline, retrying")
	case !s.lastPoll.IsZero():
		parts = append(parts, "polled "+fmtAge(now.Sub(s.lastPoll))+" ago")
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
