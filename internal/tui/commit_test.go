package tui

import (
	"strings"
	"testing"
)

const commitURL = "https://github.com/acme/api/commit/abc1234ffff0000"

func TestRunsCommitColumnIsAHyperlink(t *testing.T) {
	ctx, _ := testContext(seedStore())
	v := NewRuns("acme/api").View(ctx, 120, 6)
	if !strings.Contains(plain(v), "COMMIT") || !strings.Contains(plain(v), "abc1234") {
		t.Fatalf("no commit column:\n%s", plain(v))
	}
	if !strings.Contains(v, "\x1b]8;;"+commitURL) {
		t.Fatalf("short SHA is not an OSC 8 link to %s:\n%q", commitURL, v)
	}
	assertFits(t, v, 120)
}

func TestCommitKeyOpensCommitPage(t *testing.T) {
	ctx, rec, _ := actCtx(t)
	for _, s := range []Screen{
		NewRuns("acme/api"),
		NewJobs(run12(ctx.Store)),
		NewTail("acme", "api", ctx.Store.Jobs(12)[0], 0),
	} {
		rec.opened = nil
		_, cmd := s.Update(key("C"), ctx)
		if cmd == nil {
			t.Fatalf("%s: no command", s.Title())
		}
		if res, _ := cmd().(ActionResult); res.Err != nil {
			t.Fatalf("%s: %v", s.Title(), res.Err)
		}
		if len(rec.opened) != 1 || rec.opened[0] != commitURL {
			t.Fatalf("%s: opened %v", s.Title(), rec.opened)
		}
	}
}

func TestCommitKeyWithoutCommit(t *testing.T) {
	ctx, rec, _ := actCtx(t)
	s, _ := press(NewRuns("acme/api"), ctx, "G") // run 10 has no head SHA
	_, cmd := s.Update(key("C"), ctx)
	res, _ := cmd().(ActionResult)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "no commit for this run") || len(rec.opened) != 0 {
		t.Fatalf("res %+v opened %v", res, rec.opened)
	}
}

func TestPipelineHeaderShowsCommit(t *testing.T) {
	ctx, _ := testContext(seedStore())
	seedJobs(ctx.Store)
	_, stats := headerOf(t, NewJobs(run12(ctx.Store)), ctx)
	if !strings.Contains(stats, "abc1234 Add login form") {
		t.Fatalf("stats:\n%s", stats)
	}
}

func TestHelpListsCommitKey(t *testing.T) {
	if !strings.Contains(strings.Join(helpLines, "\n"), "  c ") {
		t.Fatal("help does not mention c")
	}
}

func TestRunsKeyColumnsSurviveNarrowTerminals(t *testing.T) {
	ctx, _ := testContext(seedStore())
	runs := ctx.Store.Runs("acme/api")
	runs[0].WorkflowName = strings.Repeat("Very long workflow name ", 3)
	runs[0].Actor = "github-actions[bot]"
	ctx.Store.SetRuns("acme/api", runs, "")
	v := plain(NewRuns("acme/api").View(ctx, 80, 6))
	for _, want := range []string{"#41", "abc1234", "3m0s"} {
		if !strings.Contains(v, want) {
			t.Errorf("80 columns lost %q:\n%s", want, v)
		}
	}
}
