// Package tail turns GitHub job logs into an incremental stream of lines.
package tail

import (
	"bytes"
	"strings"
	"time"

	"github.com/rnjn/ghat/internal/gh"
)

// Kind classifies a log line by its workflow-command marker.
type Kind int

const (
	Plain Kind = iota
	Group
	EndGroup
	Error
	Warning
	Command
)

var kindNames = [...]string{"plain", "group", "endgroup", "error", "warning", "command"}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "unknown"
}

// MarshalText makes Kind render as its name in JSON.
func (k Kind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

// LogLine is one parsed line of a job log. Text has the timestamp and the
// ##[marker] removed; Kind records which marker it had.
type LogLine struct {
	Timestamp  time.Time `json:"timestamp,omitzero"`
	Text       string    `json:"text"`
	Kind       Kind      `json:"kind"`
	StepNumber int       `json:"step"`
}

var markers = []struct {
	prefix string
	kind   Kind
}{
	{"##[group]", Group},
	{"##[endgroup]", EndGroup},
	{"##[error]", Error},
	{"##[warning]", Warning},
	{"##[command]", Command},
}

// ParseLines splits a raw job log into lines and parses each one. Steps,
// when given, are used to attribute lines to step numbers.
func ParseLines(raw []byte, steps []gh.Step) []LogLine {
	raw = bytes.TrimPrefix(raw, []byte("\uFEFF"))
	raw = bytes.TrimSuffix(raw, []byte("\n"))
	if len(raw) == 0 {
		return []LogLine{}
	}
	parts := strings.Split(string(raw), "\n")
	out := make([]LogLine, 0, len(parts))
	attr := stepAttributor{steps: steps}
	for _, p := range parts {
		l := parseLine(strings.TrimSuffix(p, "\r"))
		l.StepNumber = attr.step(l)
		out = append(out, l)
	}
	return out
}

func parseLine(s string) LogLine {
	var l LogLine
	if i := strings.IndexByte(s, ' '); i > 0 {
		if ts, err := time.Parse(time.RFC3339Nano, s[:i]); err == nil {
			l.Timestamp = ts
			s = s[i+1:]
		}
	}
	for _, m := range markers {
		if rest, ok := strings.CutPrefix(s, m.prefix); ok {
			l.Kind = m.kind
			s = rest
			break
		}
	}
	l.Text = s
	return l
}

// stepAttributor walks steps forward as group headers reveal step
// boundaries. A group whose title equals a later step's name jumps to that
// step. Any other "Run ..." group advances to the next step, but only once
// that step has started, so nested groups from composite actions stay put.
type stepAttributor struct {
	steps []gh.Step
	cur   int
}

func (a *stepAttributor) step(l LogLine) int {
	if len(a.steps) == 0 {
		return 0
	}
	if l.Kind == Group {
		if i := a.findLater(l.Text); i >= 0 {
			a.cur = i
		} else if strings.HasPrefix(l.Text, "Run ") && a.cur+1 < len(a.steps) {
			next := a.steps[a.cur+1]
			if !next.StartedAt.IsZero() && !l.Timestamp.IsZero() && !l.Timestamp.Before(next.StartedAt) {
				a.cur++
			}
		}
	}
	return a.steps[a.cur].Number
}

func (a *stepAttributor) findLater(title string) int {
	for i := a.cur + 1; i < len(a.steps); i++ {
		if a.steps[i].Name == title {
			return i
		}
	}
	return -1
}
