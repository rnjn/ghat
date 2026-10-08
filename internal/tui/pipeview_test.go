package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/workflow"
)

// seedPipeline gives run 12 a four-stage pipeline: lint and build, two
// test matrix jobs, a running deploy.
func seedPipeline(ctx *Context) {
	j := func(id int64, name, status, conclusion string, created, started, done time.Duration) gh.Job {
		job := gh.Job{ID: id, RunID: 12, Name: name, Status: status, Conclusion: conclusion, CreatedAt: ago(created), StartedAt: ago(started),
			HTMLURL: "https://github.com/acme/api/actions/runs/12/job/" + string(rune('0'+id-200))}
		if status == "completed" {
			job.CompletedAt = ago(done)
		}
		return job
	}
	deploy := j(205, "deploy / push", "in_progress", "", 4*time.Minute, 3*time.Minute, 0)
	deploy.Steps = []gh.Step{
		{Number: 1, Name: "Set up job", Status: "completed", Conclusion: "success", StartedAt: ago(3 * time.Minute), CompletedAt: ago(170 * time.Second)},
		{Number: 2, Name: "Push image", Status: "in_progress", StartedAt: ago(170 * time.Second)},
	}
	runs := ctx.Store.Runs("acme/api")
	for i := range runs {
		if runs[i].ID == 12 {
			runs[i].CreatedAt, runs[i].WorkflowID = ago(11*time.Minute), 7
		}
	}
	ctx.Store.SetRuns("acme/api", runs, "")
	ctx.Store.SetJobs(12, []gh.Job{
		j(201, "build", "completed", "success", 10*time.Minute, 9*time.Minute, 7*time.Minute),
		j(202, "Lint", "completed", "failure", 10*time.Minute, 9*time.Minute+30*time.Second, 8*time.Minute),
		j(203, "test (ubuntu)", "completed", "success", 7*time.Minute, 6*time.Minute, 5*time.Minute),
		j(204, "test (macos)", "completed", "success", 7*time.Minute, 5*time.Minute, 4*time.Minute),
		deploy,
	})
}

var pipelineSpecs = []workflow.Job{
	{Key: "build"}, {Key: "lint", Name: "Lint"},
	{Key: "test", Needs: []string{"build", "lint"}},
	{Key: "deploy", Needs: []string{"test"}},
}

func pipeCtx(t *testing.T) (*Context, *pipeView) {
	t.Helper()
	ctx, _ := testContext(seedStore())
	seedPipeline(ctx)
	ctx.Store.SetFocusRun(12)
	v := pushed(t, NewJobs(run12(ctx.Store)).(*jobsScreen).key("v", ctx)).(*pipeView)
	v.specs = pipelineSpecs
	return ctx, v
}

func (j *jobsScreen) key(k string, ctx *Context) tea.Cmd {
	_, cmd := j.Update(key(k), ctx)
	return cmd
}

func TestPipelineViewDAGGolden(t *testing.T) {
	ctx, v := pipeCtx(t)
	out := v.View(ctx, 100, 14)
	assertFits(t, out, 100)
	golden.RequireEqual(t, plain(out))
}

func TestPipelineViewTimelineGolden(t *testing.T) {
	ctx, v := pipeCtx(t)
	v.Update(key("tab"), ctx)
	v.Update(key("G"), ctx) // deploy: running, with steps
	out := v.View(ctx, 100, 14)
	assertFits(t, out, 100)
	golden.RequireEqual(t, plain(out))
}

func TestPipelineViewHeader(t *testing.T) {
	ctx, v := pipeCtx(t)
	title, stats := headerOf(t, v, ctx)
	if title != "Graph · acme/api #41 CI" {
		t.Fatalf("title %q", title)
	}
	for _, want := range []string{"in_progress", "5 jobs · 3 columns", "critical path build › test (macos) › deploy / push (11m0s)"} {
		if !strings.Contains(stats, want) {
			t.Errorf("stats %q lack %q", stats, want)
		}
	}
	if strings.Contains(stats, "timing") {
		t.Errorf("stats %q claim inferred edges", stats)
	}
	v.Update(key("tab"), ctx)
	if title, _ = headerOf(t, v, ctx); title != "Timeline · acme/api #41 CI" {
		t.Fatalf("title %q", title)
	}
	v.specs, v.loadErr = nil, errors.New("404")
	if _, stats = headerOf(t, v, ctx); !strings.Contains(stats, "needs from timing (404)") {
		t.Errorf("stats %q", stats)
	}
}

func TestPipelineViewNavigationAndEnter(t *testing.T) {
	ctx, v := pipeCtx(t)
	sel := func() string {
		g := v.graph(ctx)
		return g.nodes[v.selected(&g)].job.Name
	}
	if sel() != "build" {
		t.Fatalf("initial %q", sel())
	}
	for k, want := range [][2]string{{"j", "Lint"}, {"l", "test (macos)"}, {"k", "test (ubuntu)"}, {"l", "deploy / push"}, {"h", "test (ubuntu)"}, {"h", "build"}, {"G", "deploy / push"}, {"g", "build"}} {
		_ = k
		v.Update(key(want[0]), ctx)
		if sel() != want[1] {
			t.Fatalf("after %s: %q, want %q", want[0], sel(), want[1])
		}
	}
	v.Update(key("tab"), ctx) // timeline: j follows start order (Lint started before build)
	v.Update(key("j"), ctx)
	if sel() != "test (ubuntu)" {
		t.Fatalf("timeline j: %q", sel())
	}
	v.Update(key("k"), ctx)
	v.Update(key("k"), ctx)
	if sel() != "Lint" {
		t.Fatalf("timeline k k: %q", sel())
	}
	_, cmd := v.Update(key("enter"), ctx)
	tl := pushed(t, cmd).(*tailScreen)
	if tl.job.ID != 202 {
		t.Fatalf("tail job %d", tl.job.ID)
	}
	if _, _, id := ctx.Store.TailJob(); id != 202 {
		t.Fatalf("tail job = %d", id)
	}
}

func TestPipelineViewOpensFromRunsAndRestoresFocus(t *testing.T) {
	ctx, _, fa := actCtx(t)
	seedPipeline(ctx)
	fa.graph = pipelineSpecs
	s := NewRuns("acme/api")
	_, cmd := s.Update(key("v"), ctx)
	var v *pipeView
	var loaded []tea.Msg
	for _, m := range runCmd(cmd) {
		if p, ok := m.(Push); ok {
			v = p.Screen.(*pipeView)
		} else {
			loaded = append(loaded, m)
		}
	}
	if v == nil || !v.loading || ctx.Store.FocusRun() != 12 {
		t.Fatalf("view %+v focus %d", v, ctx.Store.FocusRun())
	}
	for _, m := range loaded {
		v.Update(m, ctx)
	}
	if len(fa.calls) == 0 || fa.calls[len(fa.calls)-1] != "graph acme/api 7@abc1234ffff0000" {
		t.Fatalf("calls %v", fa.calls)
	}
	if v.loading || len(v.specs) != 4 {
		t.Fatalf("after load: loading=%v specs=%d", v.loading, len(v.specs))
	}
	v.OnPop(ctx)
	if ctx.Store.FocusRun() != 0 {
		t.Fatalf("focus after pop %d", ctx.Store.FocusRun())
	}
}

func TestPipelineViewFromPipelineKeepsFocus(t *testing.T) {
	ctx, v := pipeCtx(t)
	v.OnPop(ctx)
	if ctx.Store.FocusRun() != 12 {
		t.Fatalf("focus after pop %d", ctx.Store.FocusRun())
	}
	if v.Resource() != "jobs:12" {
		t.Fatalf("resource %q", v.Resource())
	}
}

func TestPipelineViewLoadingAndSmall(t *testing.T) {
	ctx, _ := testContext(seedStore())
	v := &pipeView{run: run12(ctx.Store)}
	if !strings.Contains(plain(v.View(ctx, 80, 5)), "loading jobs") {
		t.Fatal("no loading message")
	}
	ctx, v = pipeCtx(t)
	for _, w := range []int{30, 50, 70} {
		for _, mode := range []viewMode{modeDAG, modeTimeline} {
			v.mode = mode
			assertFits(t, v.View(ctx, w, 6), w)
		}
	}
	_ = v.View(ctx, 0, 0)
}

func TestPipelineViewScrollsToSelection(t *testing.T) {
	ctx, v := pipeCtx(t)
	v.Update(key("G"), ctx) // deploy, in the third column
	out := plain(v.View(ctx, 40, 6))
	if !strings.Contains(out, "deploy") {
		t.Fatalf("selection scrolled out of view:\n%s", out)
	}
}

func TestPipelineViewLinks(t *testing.T) {
	ctx, rec, _ := actCtx(t)
	seedPipeline(ctx)
	v := &pipeView{run: run12(ctx.Store)}
	_, cmd := v.Update(key("O"), ctx)
	cmd()
	if len(rec.opened) != 1 || !strings.HasSuffix(rec.opened[0], "/job/1") {
		t.Fatalf("opened %v", rec.opened)
	}
}
