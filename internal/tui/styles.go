package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	colorGreen  = lipgloss.Color("2")
	colorRed    = lipgloss.Color("1")
	colorYellow = lipgloss.Color("3")
	colorBlue   = lipgloss.Color("4")
	colorDim    = lipgloss.Color("8")

	styleHeader  = lipgloss.NewStyle().Bold(true)
	styleDim     = lipgloss.NewStyle().Foreground(colorDim)
	styleError   = lipgloss.NewStyle().Foreground(colorRed)
	styleWarning = lipgloss.NewStyle().Foreground(colorYellow)
	styleStatus  = lipgloss.NewStyle().Reverse(true)
	styleGroup   = lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	styleCursor  = lipgloss.NewStyle().Reverse(true)
	styleMatch   = lipgloss.NewStyle().Background(colorYellow).Foreground(lipgloss.Color("0"))
)

// glyph is a one-cell status symbol for a run, job or step.
func glyph(status, conclusion string) string {
	switch status {
	case "in_progress":
		return lipgloss.NewStyle().Foreground(colorYellow).Render("●")
	case "queued", "waiting", "pending", "requested":
		return styleDim.Render("○")
	}
	switch conclusion {
	case "success":
		return lipgloss.NewStyle().Foreground(colorGreen).Render("✓")
	case "failure", "timed_out", "startup_failure":
		return styleError.Render("✗")
	case "cancelled":
		return styleDim.Render("⊘")
	case "skipped", "neutral", "stale":
		return styleDim.Render("–")
	case "action_required":
		return styleWarning.Render("!")
	}
	return styleDim.Render("?")
}

// state is the conclusion of a finished item, else its status.
func state(status, conclusion string) string {
	if status == "completed" && conclusion != "" {
		return conclusion
	}
	return status
}

// fmtDur renders a duration to the second, e.g. 1m30s.
func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return d.Truncate(time.Second).String()
}

// span is the duration from start to end, or to now when end is zero.
func span(start, end, now time.Time) time.Duration {
	if start.IsZero() {
		return 0
	}
	if end.IsZero() || end.Before(start) {
		end = now
	}
	return end.Sub(start)
}

func isFailure(conclusion string) bool {
	return conclusion == "failure" || conclusion == "timed_out" || conclusion == "startup_failure"
}

// fmtAge renders a coarse age: 45s, 3m, 2h, 4d.
func fmtAge(d time.Duration) string {
	switch {
	case d < 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// fit truncates every line to width and pads or cuts to exactly height lines.
func fit(lines []string, width, height int) string {
	if height <= 0 {
		return ""
	}
	out := make([]string, height)
	for i := range out {
		if i < len(lines) {
			out[i] = truncate(lines[i], width)
		}
	}
	return strings.Join(out, "\n")
}

func stripForWidth(s string) string { return ansi.Strip(s) }

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "…")
}

// maxColWidth caps a table column so long names cannot push later columns
// off screen.
const maxColWidth = 40

// table renders a header and rows with left-aligned columns, each at most
// maxColWidth wide. The row at selected gets a marker. Cells may contain
// ANSI styling.
func table(headers []string, rows [][]string, selected int) []string {
	capped := make([][]string, len(rows))
	for i, r := range rows {
		capped[i] = make([]string, len(r))
		for j, c := range r {
			capped[i][j] = truncate(c, maxColWidth)
		}
	}
	rows = capped
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = ansi.StringWidth(h)
	}
	for _, r := range rows {
		for i, c := range r {
			widths[i] = min(maxColWidth, max(widths[i], ansi.StringWidth(c)))
		}
	}
	line := func(cells []string) string {
		var b strings.Builder
		for i, c := range cells {
			b.WriteString(c)
			if i < len(cells)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-ansi.StringWidth(c)+2))
			}
		}
		return b.String()
	}
	out := []string{"  " + styleHeader.Render(line(headers))}
	for i, r := range rows {
		prefix := "  "
		if i == selected {
			prefix = "› "
		}
		out = append(out, prefix+line(r))
	}
	return out
}
