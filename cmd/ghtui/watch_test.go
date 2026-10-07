package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func runJSON(status, conclusion string) map[string]any {
	r := map[string]any{"id": 9, "run_number": 3, "name": "CI", "status": status,
		"repository": map[string]any{"name": "api", "owner": map[string]any{"login": "acme"}}}
	if conclusion != "" {
		r["conclusion"] = conclusion
	}
	return r
}

func jobJSON(id int, name, status, conclusion string) map[string]any {
	j := map[string]any{"id": id, "run_id": 9, "name": name, "status": status}
	if conclusion != "" {
		j["conclusion"] = conclusion
	}
	return j
}

func watchScript(final string) *fakeGH {
	return &fakeGH{
		runs: []map[string]any{
			runJSON("in_progress", ""),
			runJSON("in_progress", ""), // unchanged poll: silent
			runJSON("completed", final),
		},
		jobsSeq: [][]map[string]any{
			{jobJSON(1, "build", "in_progress", "")},
			{jobJSON(1, "build", "in_progress", "")},
			{jobJSON(1, "build", "completed", final)},
		},
	}
}

func TestWatchPrintsTransitions(t *testing.T) {
	ts := testNow.Local().Format("15:04:05")
	out, _, err := runCLI(t, testDeps(), serve(t, watchScript("success").handler(t)), "watch", "9")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := strings.Join([]string{
		ts + " run in_progress", // first poll: run before jobs
		ts + " job build in_progress",
		ts + " job build completed success", // later: jobs before run
		ts + " run completed success",
	}, "\n") + "\n"
	if out != want {
		t.Fatalf("stdout:\n%s\nwant:\n%s", out, want)
	}
}

func TestWatchExitCodes(t *testing.T) {
	for conclusion, code := range map[string]int{"success": 0, "failure": 1} {
		_, _, err := runCLI(t, testDeps(), serve(t, watchScript(conclusion).handler(t)), "watch", "9")
		if exitCode(err) != code {
			t.Errorf("%s: exit %d (err %v), want %d", conclusion, exitCode(err), err, code)
		}
	}
}

func TestWatchInterval(t *testing.T) {
	var slept []time.Duration
	d := testDeps()
	d.sleep = func(ctx context.Context, dur time.Duration) error {
		slept = append(slept, dur)
		return nil
	}
	_, _, err := runCLI(t, d, serve(t, watchScript("success").handler(t)), "watch", "9", "--interval", "2s")
	if err != nil {
		t.Fatal(err)
	}
	if len(slept) != 2 {
		t.Fatalf("slept %v, want 2 sleeps", slept)
	}
	for _, s := range slept {
		if s != 2*time.Second {
			t.Fatalf("slept %v, want 2s each", slept)
		}
	}
}

func TestWatchUnknownRunExits2(t *testing.T) {
	_, _, err := runCLI(t, testDeps(), serve(t, (&fakeGH{}).handler(t)), "watch", "9")
	if exitCode(err) != 2 {
		t.Fatalf("exit %d, err %v", exitCode(err), err)
	}
}

func TestWatchRetriesTransientErrors(t *testing.T) {
	f := watchScript("success")
	calls := 0
	h := f.handler(t)
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 { // first ListJobs fails
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		h(w, r)
	})
	if _, _, err := runCLI(t, testDeps(), srv, "watch", "9"); err != nil {
		t.Fatalf("err = %v", err)
	}
}
