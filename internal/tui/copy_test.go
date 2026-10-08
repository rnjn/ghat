package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// copied runs cmd and reports the URL it put on the clipboard and the text
// it flashed.
func copied(t *testing.T, cmd tea.Cmd, url string) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command")
	}
	want := tea.SetClipboard(url)()
	var gotClip, gotFlash bool
	for _, m := range runCmd(cmd) {
		if reflect.DeepEqual(m, want) {
			gotClip = true
		}
		if r, ok := m.(ActionResult); ok && r.Err == nil && r.Text == "copied "+url {
			gotFlash = true
		}
	}
	if !gotClip || !gotFlash {
		t.Fatalf("clipboard set=%v flash=%v for %s", gotClip, gotFlash, url)
	}
}

func TestCopyCommitLink(t *testing.T) {
	ctx, rec, _ := actCtx(t)
	for _, s := range []Screen{
		NewRuns("acme/api"),
		NewJobs(run12(ctx.Store)),
		NewTail("acme", "api", ctx.Store.Jobs(12)[0], 0),
	} {
		_, cmd := s.Update(key("C"), ctx)
		copied(t, cmd, commitURL)
	}
	if len(rec.opened) != 0 {
		t.Fatalf("C opened a browser: %v", rec.opened)
	}
}

func TestCopyCommitLinkWithoutCommit(t *testing.T) {
	ctx, _, _ := actCtx(t)
	s, _ := press(NewRuns("acme/api"), ctx, "G")
	_, cmd := s.Update(key("C"), ctx)
	if res, _ := cmd().(ActionResult); res.Err == nil || !strings.Contains(res.Err.Error(), "no commit for this run") {
		t.Fatalf("res %+v", res)
	}
}

func TestCopyCurrentLink(t *testing.T) {
	ctx, rec, _ := actCtx(t)
	jobs := ctx.Store.Jobs(12)
	for _, tc := range []struct {
		s    Screen
		want string
	}{
		{NewBoard(), "https://github.com/acme/api/actions"},
		{NewRuns("acme/api"), "https://github.com/acme/api/actions/runs/12"},
		{NewJobs(run12(ctx.Store)), "https://github.com/acme/api/actions/runs/12/job/120"},
		{NewTail("acme", "api", jobs[0], 0), "https://github.com/acme/api/actions/runs/12/job/120"},
		{NewTail("acme", "api", jobs[1], 0), "https://github.com/acme/api/actions/runs/12"},
	} {
		_, cmd := tc.s.Update(key("O"), ctx)
		copied(t, cmd, tc.want)
	}
	if len(rec.opened) != 0 {
		t.Fatalf("O opened a browser: %v", rec.opened)
	}
}

func TestHelpListsCopyKeys(t *testing.T) {
	help := strings.Join(helpLines, "\n")
	if !strings.Contains(help, "  C ") || !strings.Contains(help, "  O ") {
		t.Fatalf("help:\n%s", help)
	}
}
