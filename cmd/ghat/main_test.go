package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func runMain(t *testing.T, d *deps, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(d, args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRunVersionExits0(t *testing.T) {
	code, out, errOut := runMain(t, testDeps(), "--version")
	if code != 0 || !strings.HasPrefix(strings.TrimSpace(out), "ghat dev") || errOut != "" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
}

func TestRunUsageErrorPrintsOnceAndExits2(t *testing.T) {
	code, _, errOut := runMain(t, testDeps(), "--no-such-flag")
	if code != 2 || strings.Count(errOut, "ghat:") != 1 || !strings.Contains(errOut, "no-such-flag") {
		t.Fatalf("code %d stderr %q", code, errOut)
	}
}

func TestRunAPIErrorPrintsOnceAndExits2(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	})
	code, _, errOut := runMain(t, testDeps(), "--api-url", srv.URL, "list")
	if code != 2 || strings.Count(errOut, "ghat:") != 1 || !strings.Contains(errOut, "500 boom") {
		t.Fatalf("code %d stderr %q", code, errOut)
	}
}

func TestRunFailedConclusionIsSilentExit1(t *testing.T) {
	f := growingJob("failure")
	srv := serve(t, f.handler(t))
	code, out, errOut := runMain(t, testDeps(), "--api-url", srv.URL, "tail", "12")
	if code != 1 || strings.Contains(errOut, "ghat:") || !strings.Contains(out, "pkg/b failed") {
		t.Fatalf("code %d out %q stderr %q", code, out, errOut)
	}
}

func TestRunAmbiguousRunPrintsJobListOnlyAndExits2(t *testing.T) {
	f := &fakeGH{jobID: 12, runJobs: map[int64][]map[string]any{9: {
		{"id": 21, "name": "lint", "status": "in_progress"},
		{"id": 22, "name": "test", "status": "in_progress"},
	}}}
	code, _, errOut := runMain(t, testDeps(), "--api-url", serve(t, f.handler(t)).URL, "tail", "9")
	if code != 2 || strings.Contains(errOut, "ghat:") || !strings.Contains(errOut, "22  test") {
		t.Fatalf("code %d stderr %q", code, errOut)
	}
}
