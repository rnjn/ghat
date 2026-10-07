package tui

import (
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

	vis        []int // indices of displayable lines (end-group markers hidden)
	indexed    int   // buffer lines already examined for vis
	offset     int
	follow     bool
	timestamps bool
	logErr     string
	lastH      int
}

// NewTail opens job; step, when non-zero, is the step to scroll to.
func NewTail(owner, repo string, job gh.Job, step int) Screen {
	return &tailScreen{owner: owner, repo: repo, job: job, step: step, follow: step == 0}
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
	case tea.KeyPressMsg:
		if cmd := runKey(ctx, m.String(), t.job.RunID); cmd != nil {
			return t, cmd
		}
		t.key(m.String())
	}
	return t, nil
}

func (t *tailScreen) key(k string) {
	page := max(1, t.lastH-1)
	switch k {
	case "t":
		t.timestamps = !t.timestamps
	case "G", "end":
		t.follow = true
	case "g", "home":
		t.follow, t.offset = false, 0
	case "j", "down":
		t.scroll(1)
	case "k", "up":
		t.scroll(-1)
	case "pgdown", "ctrl+f", " ":
		t.scroll(page)
	case "pgup", "ctrl+b":
		t.scroll(-page)
	}
}

func (t *tailScreen) scroll(delta int) {
	if t.follow {
		t.offset = t.maxOffset()
	}
	t.follow = false
	t.offset = max(0, min(t.offset+delta, t.maxOffset()))
}

func (t *tailScreen) maxOffset() int { return max(0, len(t.vis)-t.lastH) }

// refresh picks up the latest job state and indexes new log lines.
func (t *tailScreen) refresh(ctx *Context) []tail.LogLine {
	for _, j := range ctx.Store.Jobs(t.job.RunID) {
		if j.ID == t.job.ID {
			t.job = j
		}
	}
	lines := ctx.Store.Log(t.job.ID).Lines
	if len(lines) < t.indexed {
		t.vis, t.indexed = nil, 0
	}
	for ; t.indexed < len(lines); t.indexed++ {
		if lines[t.indexed].Kind != tail.EndGroup {
			t.vis = append(t.vis, t.indexed)
		}
	}
	return lines
}

func (t *tailScreen) View(ctx *Context, width, height int) string {
	lines := t.refresh(ctx)
	if height <= 0 || width <= 0 {
		return ""
	}
	bodyH := height - 1
	t.lastH = bodyH
	now := ctx.Now()
	if len(t.vis) == 0 {
		return fit(append(t.waitingBody(ctx), t.spinnerFooter(now)), width, height)
	}
	if t.step > 0 {
		for i, idx := range t.vis {
			if lines[idx].StepNumber == t.step {
				t.offset = i
				break
			}
		}
		t.step = 0
	}
	if t.follow {
		t.offset = t.maxOffset()
	}
	t.offset = max(0, min(t.offset, t.maxOffset()))
	end := min(len(t.vis), t.offset+bodyH)
	out := make([]string, 0, height)
	for _, idx := range t.vis[t.offset:end] {
		out = append(out, t.render(lines[idx]))
	}
	for len(out) < bodyH {
		out = append(out, "")
	}
	mode := "paused (G to follow)"
	if t.follow {
		mode = "following"
	}
	footer := styleDim.Render(fmt.Sprintf(" lines %d–%d of %d · %s", t.offset+1, end, len(t.vis), mode))
	if t.job.Status != "completed" {
		footer = t.spinnerFooter(now)
	}
	return fit(append(out, footer), width, height)
}

func (t *tailScreen) render(l tail.LogLine) string {
	text := sanitize(l.Text)
	var s string
	switch l.Kind {
	case tail.Group:
		s = styleGroup.Render("▸ " + text)
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
