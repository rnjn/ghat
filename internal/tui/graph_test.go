package tui

import (
	"reflect"
	"testing"
	"time"

	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/workflow"
)

func doneJob(id int64, name string, start, end time.Duration) gh.Job {
	return gh.Job{ID: id, Name: name, Status: "completed", Conclusion: "success", StartedAt: ago(start), CompletedAt: ago(end)}
}

func TestBuildGraphFromNeeds(t *testing.T) {
	jobs := []gh.Job{
		doneJob(1, "build", 10*time.Minute, 8*time.Minute),
		doneJob(2, "Lint", 10*time.Minute, 9*time.Minute),
		doneJob(3, "test (ubuntu)", 8*time.Minute, 5*time.Minute),
		doneJob(4, "test (macos)", 8*time.Minute, 4*time.Minute),
		doneJob(5, "deploy / push", 4*time.Minute, 3*time.Minute),
	}
	specs := []workflow.Job{
		{Key: "build"}, {Key: "lint", Name: "Lint"},
		{Key: "test", Needs: []string{"build", "lint"}},
		{Key: "deploy", Needs: []string{"test"}},
	}
	g := buildGraph(jobs, specs, testNow, false)
	if g.inferred {
		t.Fatal("edges should come from needs")
	}
	cols := make([]int, len(g.nodes))
	for i, n := range g.nodes {
		cols[i] = n.col
	}
	if want := []int{0, 0, 1, 1, 2}; !reflect.DeepEqual(cols, want) {
		t.Fatalf("cols %v want %v", cols, want)
	}
	if want := []int{0, 1}; !reflect.DeepEqual(g.nodes[2].needs, want) {
		t.Fatalf("test needs %v", g.nodes[2].needs)
	}
	if want := []int{2, 3}; !reflect.DeepEqual(g.nodes[4].needs, want) {
		t.Fatalf("deploy needs %v", g.nodes[4].needs)
	}
	// deploy finished last; its latest-finishing need is test (macos); then build.
	if want := []int{0, 3, 4}; !reflect.DeepEqual(g.critical, want) {
		t.Fatalf("critical %v want %v", g.critical, want)
	}
	if len(g.cols) != 3 || !reflect.DeepEqual(g.cols[1], []int{2, 3}) {
		t.Fatalf("cols %v", g.cols)
	}
}

func TestBuildGraphInfersFromTiming(t *testing.T) {
	jobs := []gh.Job{
		doneJob(1, "a", 10*time.Minute, 9*time.Minute),
		doneJob(2, "b", 9*time.Minute, 7*time.Minute),
		doneJob(3, "c", 10*time.Minute, 8*time.Minute),
		doneJob(4, "d", 7*time.Minute, 6*time.Minute),
		{ID: 5, Name: "e", Status: "queued"},
	}
	g := buildGraph(jobs, nil, testNow, false)
	if !g.inferred {
		t.Fatal("should be inferred")
	}
	// b after a; d after b and c (a is transitively before d, so not an edge); e waits on everything done.
	if !reflect.DeepEqual(g.nodes[1].needs, []int{0}) || !reflect.DeepEqual(g.nodes[3].needs, []int{1, 2}) {
		t.Fatalf("needs b=%v d=%v", g.nodes[1].needs, g.nodes[3].needs)
	}
	if g.nodes[4].col != 3 || g.nodes[3].col != 2 {
		t.Fatalf("cols d=%d e=%d", g.nodes[3].col, g.nodes[4].col)
	}
}

func TestBuildGraphUnmatchedJobFallsBackToTiming(t *testing.T) {
	jobs := []gh.Job{doneJob(1, "build", 10*time.Minute, 8*time.Minute), doneJob(2, "Test on linux", 8*time.Minute, 5*time.Minute)}
	specs := []workflow.Job{{Key: "build"}, {Key: "test", Name: "Test on ${{ matrix.os }}", Needs: []string{"build"}}}
	g := buildGraph(jobs, specs, testNow, false)
	if !g.inferred || !reflect.DeepEqual(g.nodes[1].needs, []int{0}) {
		t.Fatalf("inferred=%v needs=%v", g.inferred, g.nodes[1].needs)
	}
}

func TestBuildGraphSurvivesCycle(t *testing.T) {
	jobs := []gh.Job{doneJob(1, "a", time.Minute, 0), doneJob(2, "b", time.Minute, 0)}
	specs := []workflow.Job{{Key: "a", Needs: []string{"b"}}, {Key: "b", Needs: []string{"a"}}}
	g := buildGraph(jobs, specs, testNow, false)
	if len(g.cols) == 0 || len(g.critical) == 0 {
		t.Fatalf("graph %+v", g)
	}
}

func TestBuildGraphEmpty(t *testing.T) {
	g := buildGraph(nil, nil, testNow, false)
	if len(g.nodes) != 0 || len(g.cols) != 0 || len(g.critical) != 0 {
		t.Fatalf("graph %+v", g)
	}
}

func TestBuildGraphAddsPendingJobsFromSpecs(t *testing.T) {
	jobs := []gh.Job{{ID: 1, Name: "build", Status: "in_progress", StartedAt: ago(time.Minute)}}
	specs := []workflow.Job{
		{Key: "build"},
		{Key: "test", Name: "Test ${{ matrix.os }}", Needs: []string{"build"}},
		{Key: "deploy", Needs: []string{"test"}},
	}
	g := buildGraph(jobs, specs, testNow, true)
	if len(g.nodes) != 3 || len(g.cols) != 3 {
		t.Fatalf("nodes %d cols %d", len(g.nodes), len(g.cols))
	}
	test, deploy := g.nodes[1], g.nodes[2]
	if !test.placeholder || test.job.Name != "test" || test.job.Status != "pending" || test.col != 1 || !reflect.DeepEqual(test.needs, []int{0}) {
		t.Fatalf("test node %+v", test)
	}
	if !deploy.placeholder || deploy.col != 2 || !reflect.DeepEqual(deploy.needs, []int{1}) {
		t.Fatalf("deploy node %+v", deploy)
	}
	if g.pending != 2 || g.inferred || !reflect.DeepEqual(g.critical, []int{0}) {
		t.Fatalf("pending %d inferred %v critical %v", g.pending, g.inferred, g.critical)
	}
	if g := buildGraph(jobs, specs, testNow, false); len(g.nodes) != 1 {
		t.Fatalf("finished run grew placeholders: %d nodes", len(g.nodes))
	}
}
