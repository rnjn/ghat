package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const runsBody = `{"total_count":2,"workflow_runs":[
 {"id":101,"run_number":12,"name":"CI","workflow_id":1,"head_branch":"main","event":"push","status":"in_progress","conclusion":null,
  "created_at":"2026-10-07T11:58:30Z","updated_at":"2026-10-07T11:59:00Z","html_url":"https://github.com/acme/api/actions/runs/101",
  "actor":{"login":"alice"},"repository":{"name":"api","owner":{"login":"acme"}}},
 {"id":100,"run_number":11,"name":"CI","workflow_id":1,"head_branch":"fix-x","event":"pull_request","status":"completed","conclusion":"failure",
  "created_at":"2026-10-07T10:00:00Z","updated_at":"2026-10-07T10:03:25Z","html_url":"https://github.com/acme/api/actions/runs/100",
  "actor":{"login":"bob"},"repository":{"name":"api","owner":{"login":"acme"}}}
]}`

func runsServer(t *testing.T, gotPath *string, gotQuery *map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if gotPath != nil {
			*gotPath = r.URL.Path
		}
		if gotQuery != nil {
			*gotQuery = map[string]string{}
			for k := range r.URL.Query() {
				(*gotQuery)[k] = r.URL.Query().Get(k)
			}
		}
		_, _ = w.Write([]byte(runsBody))
	}
}

func TestListTable(t *testing.T) {
	var path string
	srv := serve(t, runsServer(t, &path, nil))
	out, _, err := runCLI(t, testDeps(), srv, "list")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/repos/acme/api/actions/runs" {
		t.Errorf("repo not inferred from git remote: path %q", path)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("output:\n%s", out)
	}
	for _, col := range []string{"STATUS", "WORKFLOW", "BRANCH", "EVENT", "ACTOR", "DURATION", "RUN", "ID"} {
		if !strings.Contains(lines[0], col) {
			t.Errorf("header %q lacks %s", lines[0], col)
		}
	}
	for _, want := range []string{"in_progress", "CI", "main", "push", "alice", "1m30s", "#12", "101"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("row 1 %q lacks %q", lines[1], want)
		}
	}
	for _, want := range []string{"failure", "fix-x", "pull_request", "bob", "3m25s", "#11", "100"} {
		if !strings.Contains(lines[2], want) {
			t.Errorf("row 2 %q lacks %q", lines[2], want)
		}
	}
}

func TestListJSON(t *testing.T) {
	srv := serve(t, runsServer(t, nil, nil))
	out, _, err := runCLI(t, testDeps(), srv, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 JSON lines, got:\n%s", out)
	}
	var r struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
		Repo   string `json:"repo"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &r); err != nil {
		t.Fatal(err)
	}
	if r.ID != 101 || r.Status != "in_progress" || r.Repo != "acme/api" {
		t.Fatalf("decoded %+v", r)
	}
}

func TestListArgsAndFlags(t *testing.T) {
	var path string
	var q map[string]string
	srv := serve(t, runsServer(t, &path, &q))
	_, _, err := runCLI(t, testDeps(), srv, "list", "other/repo", "--branch", "dev", "--status", "failure", "--limit", "5")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/repos/other/repo/actions/runs" {
		t.Errorf("path = %q", path)
	}
	if q["branch"] != "dev" || q["status"] != "failure" || q["per_page"] != "5" {
		t.Errorf("query = %v", q)
	}
}

func TestListAPIErrorExits2(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})
	_, _, err := runCLI(t, testDeps(), srv, "list")
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("err = %v, exit %d", err, exitCode(err))
	}
}

func TestListBadRepoArgExits2(t *testing.T) {
	_, _, err := runCLI(t, testDeps(), nil, "list", "not-a-repo")
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("err = %v", err)
	}
}

func TestListShowsCommit(t *testing.T) {
	body := strings.Replace(runsBody, `"head_branch":"main"`, `"head_branch":"main","head_sha":"e2c23169990b8371","head_commit":{"message":"Fix it\nbody","author":{"name":"Ada"}}`, 1)
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })
	out, _, err := runCLI(t, testDeps(), srv, "list")
	if err != nil || !strings.Contains(out, "COMMIT") || !strings.Contains(out, "e2c2316") {
		t.Fatalf("err %v out:\n%s", err, out)
	}
	out, _, _ = runCLI(t, testDeps(), srv, "list", "--json")
	if !strings.Contains(out, `"head_sha":"e2c23169990b8371"`) || !strings.Contains(out, `"commit_message":"Fix it"`) {
		t.Fatalf("json:\n%s", out)
	}
}
