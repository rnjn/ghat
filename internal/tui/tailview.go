package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"ghtui/internal/gh"
	"ghtui/internal/poller"
	"ghtui/internal/tail"
)

var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// tailScreen shows one job: live steps while it runs, its log once done.
// It renders only the visible window of the log, so huge logs stay cheap.
type tailScreen struct {
	owner, repo string
	job         gh.Job
	step        int // step to scroll to once the log arrives; 0 = none

	ix         logIndex
	cur        int // cursor: index into ix.vis
	offset     int // first visible row: index into ix.vis
	follow     bool
	timestamps bool
	logErr     string
	lastH      int
	search     tailSearch
}

// NewTail opens job; step, when non-zero, is the step to scroll to.
func NewTail(owner, repo string, job gh.Job, step int) Screen {
	return &tailScreen{owner: owner, repo: repo, job: job, step: step, follow: step == 0, ix: newLogIndex()}
}

func (t *tailScreen) Title() string { return t.job.Name }

// OnPop stops polling this job's log.
func (t *tailScreen) OnPop(ctx *Context) { ctx.Store.SetTailJob("", "", 0) }

func (t *tailScreen) Update(msg tea.Msg, ctx *Context) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case poller.PollerError:
		if m.Resource == fmt.Sprintf("log:%d", t.job.ID) {
			t.logErr = "log not available yet / expired"
		}
	case poller.LogAppended:
		if m.JobID == t.job.ID {
			t.logErr = ""
		}
	case poller.JobsUpdated:
		if m.RunID == t.job.RunID && t.followRerun(ctx) {
			return t, func() tea.Msg { return ActionResult{Text: "following rerun attempt"} }
		}
	case tea.KeyPressMsg:
		lines := ctx.Store.Log(t.job.ID).Lines
		t.index(lines)
		if used, cmd := t.searchKey(m, lines); used {
			return t, cmd
		}
		if cmd := runKey(ctx, m.String(), t.job.RunID); cmd != nil {
			return t, cmd
		}
		if cmd := dispatchForRun(ctx, m.String(), t.job.RunID); cmd != nil {
			return t, cmd
		}
		if m.String() == "o" {
			url := t.job.HTMLURL
			if url == "" {
				url = runURL(ctx, t.job.RunID)
			}
			if url == "" {
				return t, fail(errors.New("no page for this job"))
			}
			return t, openURL(ctx, url)
		}
		t.key(m.String(), ctx.Store.Log(t.job.ID).Lines)
	}
	return t, nil
}

func (t *tailScreen) key(k string, lines []tail.LogLine) {
	page := max(1, t.lastH-1)
	switch k {
	case "t":
		t.timestamps = !t.timestamps
	case "G", "end":
		t.follow = true
	case "g", "home":
		t.move(-len(t.ix.vis))
	case "j", "down":
		t.move(1)
	case "k", "up":
		t.move(-1)
	case "pgdown", "ctrl+f", " ":
		t.move(page)
	case "pgup", "ctrl+b":
		t.move(-page)
	case "z":
		t.toggleFold(lines)
	case "Z":
		clear(t.ix.folded)
		t.refold(lines, -1)
	}
}

// move shifts the cursor, leaving follow mode.
func (t *tailScreen) move(delta int) {
	if t.follow {
		t.cur = len(t.ix.vis) - 1
	}
	t.follow = false
	t.cur = max(0, min(t.cur+delta, len(t.ix.vis)-1))
}

// toggleFold folds the group under the cursor, or unfolds a folded header.
func (t *tailScreen) toggleFold(lines []tail.LogLine) {
	if len(t.ix.vis) == 0 || len(lines) < t.ix.indexed {
		return
	}
	if t.follow {
		t.cur = len(t.ix.vis) - 1
	}
	h := t.ix.header(t.ix.vis[t.cur], lines)
	if h < 0 {
		return
	}
	t.ix.folded[h] = !t.ix.folded[h]
	t.follow = false
	t.refold(lines, h)
}

// refold rebuilds the visible lines and puts the cursor on line keep (or
// keeps the current line when keep is -1).
func (t *tailScreen) refold(lines []tail.LogLine, keep int) {
	if keep < 0 && len(t.ix.vis) > 0 {
		keep = t.ix.vis[t.cur]
	}
	t.ix.rebuild(lines)
	if keep >= 0 {
		t.cur = t.ix.position(keep)
	}
}

// followRerun switches to the same-named job of a new run attempt when the
// open job has left its run's job list. It reports whether it switched.
func (t *tailScreen) followRerun(ctx *Context) bool {
	jobs := ctx.Store.Jobs(t.job.RunID)
	for _, j := range jobs {
		if j.ID == t.job.ID {
			return false
		}
	}
	for _, j := range jobs {
		if j.Name == t.job.Name {
			t.job = j
			t.ix, t.search = newLogIndex(), tailSearch{}
			t.cur, t.offset, t.follow, t.logErr = 0, 0, true, ""
			ctx.Store.SetTailJob(t.owner, t.repo, j.ID)
			return true
		}
	}
	return false
}

// refresh picks up the latest job state and indexes new log lines.
func (t *tailScreen) refresh(ctx *Context) []tail.LogLine {
	for _, j := range ctx.Store.Jobs(t.job.RunID) {
		if j.ID == t.job.ID {
			t.job = j
		}
	}
	lines := ctx.Store.Log(t.job.ID).Lines
	t.index(lines)
	return lines
}

// index brings the fold and search indexes up to date. If the log was
// replaced by a shorter one, everything that points into it starts over.
func (t *tailScreen) index(lines []tail.LogLine) {
	if len(lines) < t.ix.indexed {
		t.ix, t.search = newLogIndex(), tailSearch{}
		t.cur, t.offset, t.follow = 0, 0, true
	}
	t.ix.add(lines)
	t.search.index(lines)
}

func (t *tailScreen) View(ctx *Context, width, height int) string {
	lines := t.refresh(ctx)
	if height <= 0 || width <= 0 {
		return ""
	}
	bodyH := height - 1
	t.lastH = bodyH
	now := ctx.Now()
	if len(t.ix.vis) == 0 {
		return fit(append(t.waitingBody(ctx), t.spinnerFooter(now)), width, height)
	}
	vis := t.ix.vis
	if t.step > 0 {
		for i, idx := range vis {
			if lines[idx].StepNumber == t.step {
				t.cur, t.offset = i, i
				break
			}
		}
		t.step = 0
	}
	if t.follow {
		t.cur = len(vis) - 1
	}
	t.cur = max(0, min(t.cur, len(vis)-1))
	maxOff := max(0, len(vis)-bodyH)
	if t.cur < t.offset {
		t.offset = t.cur
	}
	if t.cur >= t.offset+bodyH {
		t.offset = t.cur - bodyH + 1
	}
	if t.follow {
		t.offset = maxOff
	}
	t.offset = max(0, min(t.offset, maxOff))
	end := min(len(vis), t.offset+bodyH)
	out := make([]string, 0, height)
	for i := t.offset; i < end; i++ {
		l := t.render(lines, vis[i])
		if i == t.cur && !t.follow {
			l = styleCursor.Render(ansi.Strip(l))
		}
		out = append(out, l)
	}
	for len(out) < bodyH {
		out = append(out, "")
	}
	mode := "paused (G to follow)"
	if t.follow {
		mode = "following"
	}
	info := fmt.Sprintf(" lines %d–%d of %d · %s", t.offset+1, end, len(vis), mode)
	if st := t.search.status(); st != "" {
		info = " " + st + " ·" + info
	}
	footer := styleDim.Render(info)
	if t.search.open {
		footer = " " + t.search.status()
	}
	if t.job.Status != "completed" {
		footer = t.spinnerFooter(now)
	}
	return fit(append(out, footer), width, height)
}

func (t *tailScreen) render(lines []tail.LogLine, i int) string {
	l := lines[i]
	text := sanitize(l.Text)
	if hl, ok := t.search.highlight(text); ok {
		prefix := kindPrefix(l.Kind)
		if l.Kind == tail.Group && t.ix.folded[i] {
			prefix = "▸ "
		}
		s := prefix + hl
		if t.timestamps && !l.Timestamp.IsZero() {
			s = styleDim.Render(l.Timestamp.Local().Format("15:04:05")) + " " + s
		}
		return s
	}
	var s string
	switch l.Kind {
	case tail.Group:
		if t.ix.folded[i] {
			s = styleGroup.Render(fmt.Sprintf("▸ %s (%d lines)", text, t.ix.count[i]))
		} else {
			s = styleGroup.Render("▾ " + text)
		}
	case tail.Error:
		s = styleError.Render("error: " + text)
	case tail.Warning:
		s = styleWarning.Render("warning: " + text)
	case tail.Command:
		s = styleDim.Render("$ " + text)
	default:
		s = text
	}
	if t.timestamps && !l.Timestamp.IsZero() {
		s = styleDim.Render(l.Timestamp.Local().Format("15:04:05")) + " " + s
	}
	return s
}

// waitingBody is shown before any log line exists.
func (t *tailScreen) waitingBody(ctx *Context) []string {
	switch {
	case t.logErr != "":
		return []string{"", "  " + styleWarning.Render(t.logErr)}
	case t.job.Status == "completed":
		return []string{"", "  fetching log…"}
	}
	now := ctx.Now()
	var out []string
	for _, st := range t.job.Steps {
		dur := ""
		if !st.StartedAt.IsZero() {
			dur = fmtDur(span(st.StartedAt, st.CompletedAt, now))
		}
		out = append(out, fmt.Sprintf("  %s %-30s %s", glyph(st.Status, st.Conclusion), st.Name, dur))
	}
	return append(out, "", "  "+styleDim.Render("log is published when the job finishes"))
}

// spinnerFooter names the running step and its elapsed time.
func (t *tailScreen) spinnerFooter(now time.Time) string {
	if t.job.Status == "completed" {
		return ""
	}
	frame := string(spinnerFrames[int(now.UnixMilli()/100)%len(spinnerFrames)])
	for _, st := range t.job.Steps {
		if st.Status == "in_progress" {
			return fmt.Sprintf(" %s %s · %s", frame, st.Name, fmtDur(span(st.StartedAt, st.CompletedAt, now)))
		}
	}
	return fmt.Sprintf(" %s %s", frame, strings.ReplaceAll(t.job.Status, "_", " "))
}

// Resource is what R re-polls.
func (t *tailScreen) Resource() string { return fmt.Sprintf("log:%d", t.job.ID) }

// sanitize makes raw log text safe to lay out: escape sequences removed,
// carriage-return overwrites resolved to the final text, tabs expanded to
// 8-column stops, and remaining control characters dropped.
func sanitize(s string) string {
	s = ansi.Strip(s)
	if i := strings.LastIndexByte(s, '\r'); i >= 0 {
		s = s[i+1:]
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		switch {
		case r == '\t':
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
			col += n
		case r < 0x20 || r == 0x7f:
		default:
			b.WriteRune(r)
			col += ansi.StringWidth(string(r))
		}
	}
	return b.String()
}

// kindPrefix is the plain-text marker for a line kind.
func kindPrefix(k tail.Kind) string {
	switch k {
	case tail.Group:
		return "▾ "
	case tail.Error:
		return "error: "
	case tail.Warning:
		return "warning: "
	case tail.Command:
		return "$ "
	}
	return ""
}
