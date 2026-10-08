package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghat/internal/tail"
)

// tailSearch is the Tail screen's / search: literal, case-insensitive.
type tailSearch struct {
	open  bool
	input textinput.Model
	query string   // applied query, lower-cased
	hits  []int    // matching line indices, ascending
	hit   int      // current hit
	lower []string // lower-cased sanitised text per line, built incrementally
}

// escHandler is implemented by screens that use esc themselves (closing a
// search or filter) before the model would pop them.
type escHandler interface{ HandleEsc() bool }

// CapturesKeys sends typed keys to the search line while it is open.
func (t *tailScreen) CapturesKeys() bool { return t.search.open }

// HandleEsc closes the search line instead of leaving the screen.
func (t *tailScreen) HandleEsc() bool {
	if t.search.open {
		t.search.open = false
		return true
	}
	return false
}

// index keeps the lower-cased copy of each line in step with the log.
func (s *tailSearch) index(lines []tail.LogLine) {
	if len(lines) < len(s.lower) {
		s.lower = nil
	}
	for i := len(s.lower); i < len(lines); i++ {
		s.lower = append(s.lower, strings.ToLower(sanitize(lines[i].Text)))
	}
}

// searchKey handles keys while the search line is open, and /, n, N.
// It reports whether it used the key.
func (t *tailScreen) searchKey(msg tea.KeyPressMsg, lines []tail.LogLine) (bool, tea.Cmd) {
	s := &t.search
	if s.open {
		if msg.String() == "enter" {
			s.open = false
			t.applySearch(strings.TrimSpace(s.input.Value()), lines)
			return true, nil
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return true, cmd
	}
	switch msg.String() {
	case "/":
		s.open = true
		s.input = textinput.New()
		s.input.Prompt = "/"
		return true, s.input.Focus()
	case "n", "N":
		if len(s.hits) == 0 {
			return s.query != "", nil
		}
		step := 1
		if msg.String() == "N" {
			step = -1
		}
		s.hit = (s.hit + step + len(s.hits)) % len(s.hits)
		t.jump(s.hits[s.hit], lines)
		return true, nil
	}
	return false, nil
}

// applySearch finds every hit and jumps to the first at or after the cursor.
func (t *tailScreen) applySearch(q string, lines []tail.LogLine) {
	s := &t.search
	s.query, s.hits, s.hit = strings.ToLower(q), nil, 0
	if s.query == "" {
		return
	}
	s.index(lines)
	for i, l := range s.lower {
		if lines[i].Kind != tail.EndGroup && strings.Contains(l, s.query) {
			s.hits = append(s.hits, i)
		}
	}
	if len(s.hits) == 0 {
		return
	}
	from := 0
	if len(t.ix.vis) > 0 && !t.follow {
		from = t.ix.vis[t.cur]
	}
	for k, h := range s.hits {
		if h >= from {
			s.hit = k
			break
		}
	}
	t.jump(s.hits[s.hit], lines)
}

// jump puts the cursor on line i, unfolding its group if needed.
func (t *tailScreen) jump(i int, lines []tail.LogLine) {
	if g := t.ix.groupOf[i]; g >= 0 && t.ix.folded[g] {
		t.ix.folded[g] = false
		t.ix.rebuild(lines)
	}
	t.follow = false
	t.cur = t.ix.position(i)
}

// highlight renders text with every occurrence of the query marked.
func (s *tailSearch) highlight(text string) (string, bool) {
	if s.query == "" {
		return text, false
	}
	low := strings.ToLower(text)
	if len(low) != len(text) || !strings.Contains(low, s.query) {
		return text, false
	}
	var b strings.Builder
	for {
		i := strings.Index(low, s.query)
		if i < 0 {
			b.WriteString(text)
			return b.String(), true
		}
		b.WriteString(text[:i])
		b.WriteString(styleMatch.Render(text[i : i+len(s.query)]))
		text, low = text[i+len(s.query):], low[i+len(s.query):]
	}
}

// status is the footer text for the search.
func (s *tailSearch) status() string {
	switch {
	case s.open:
		return s.input.View()
	case s.query == "":
		return ""
	case len(s.hits) == 0:
		return fmt.Sprintf("no matches for %s", s.query)
	}
	return fmt.Sprintf("hit %d of %d", s.hit+1, len(s.hits))
}
