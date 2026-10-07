package main

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	l1 = "2026-10-07T04:28:23.1Z ##[group]Run make test\n"
	l2 = "2026-10-07T04:28:24.2Z ok  pkg/a\n"
	l3 = "2026-10-07T04:28:25.3Z ##[error]pkg/b failed\n"
)

func growingJob(conclusion string) *fakeGH {
	return &fakeGH{
		jobID: 12,
		// The first GetJob is ResolveJob's; the tailer's polls follow.
		statuses: []string{"in_progress", "in_progress", "in_progress", "completed"},
		conc:     conclusion,
		logs:     []string{"", l1, l1 + l2 + l3},
	}
}

func TestTailFollowsNewLinesOnly(t *testing.T) {
	f := growingJob("success")
	out, _, err := runCLI(t, testDeps(), serve(t, f.handler(t)), "tail", "12")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := "▸ Run make test\nok  pkg/a\nerror: pkg/b failed\n"
	if out != want {
		t.Fatalf("stdout:\n%q\nwant:\n%q", out, want)
	}
	if f.logCalls != 3 {
		t.Fatalf("log calls = %d", f.logCalls)
	}
}

func TestTailTimestamps(t *testing.T) {
	out, _, err := runCLI(t, testDeps(), serve(t, growingJob("success").handler(t)), "tail", "12", "--timestamps")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "2026-10-07T04:28:23.1Z ▸ Run make test\n") {
		t.Fatalf("stdout:\n%s", out)
	}
}

func TestTailJSON(t *testing.T) {
	out, _, err := runCLI(t, testDeps(), serve(t, growingJob("success").handler(t)), "tail", "12", "--json")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("stdout:\n%s", out)
	}
	var l struct {
		Text string `json:"text"`
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(lines[2]), &l); err != nil {
		t.Fatal(err)
	}
	if l.Kind != "error" || l.Text != "pkg/b failed" {
		t.Fatalf("decoded %+v", l)
	}
}

func TestTailNoFollow(t *testing.T) {
	f := growingJob("success")
	f.logs = []string{l1}
	out, _, err := runCLI(t, testDeps(), serve(t, f.handler(t)), "tail", "12", "--no-follow")
	if err != nil {
		t.Fatal(err)
	}
	if out != "▸ Run make test\n" || f.logCalls != 1 {
		t.Fatalf("stdout %q, log calls %d", out, f.logCalls)
	}
}

func TestTailExitCodes(t *testing.T) {
	for conclusion, code := range map[string]int{"success": 0, "failure": 1, "cancelled": 1} {
		_, _, err := runCLI(t, testDeps(), serve(t, growingJob(conclusion).handler(t)), "tail", "12")
		if exitCode(err) != code {
			t.Errorf("%s: exit %d (err %v), want %d", conclusion, exitCode(err), err, code)
		}
	}
}

func TestTailAmbiguousRun(t *testing.T) {
	f := &fakeGH{jobID: 12, runJobs: map[int64][]map[string]any{9: {
		{"id": 21, "name": "lint", "status": "in_progress"},
		{"id": 22, "name": "test", "status": "in_progress"},
	}}}
	_, stderr, err := runCLI(t, testDeps(), serve(t, f.handler(t)), "tail", "9")
	if exitCode(err) != 2 {
		t.Fatalf("exit %d, err %v", exitCode(err), err)
	}
	for _, want := range []string{"21", "lint", "22", "test"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr %q lacks %q", stderr, want)
		}
	}
}

func TestTailRepoFlagOverridesInference(t *testing.T) {
	d := testDeps()
	d.run = func(string, ...string) ([]byte, error) {
		t.Fatal("git should not be consulted when --repo is set")
		return nil, nil
	}
	f := growingJob("success")
	_, _, err := runCLI(t, d, serve(t, f.handler(t)), "tail", "12", "--repo", "acme/api")
	if err != nil {
		t.Fatal(err)
	}
}

func TestTailBadIDExits2(t *testing.T) {
	_, _, err := runCLI(t, testDeps(), nil, "tail", "abc")
	if exitCode(err) != 2 {
		t.Fatalf("exit %d", exitCode(err))
	}
}
