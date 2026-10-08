package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// serveFixtures maps request paths to fixture files.
func serveFixtures(t *testing.T, routes map[string]string, check func(*http.Request)) *Client {
	t.Helper()
	return newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			check(r)
		}
		name, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(fixture(t, name))
	})
}

func TestListReposPaginatesUntilCutoff(t *testing.T) {
	var srv *httptest.Server
	pages := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		q := r.URL.Query()
		if q.Get("sort") != "pushed" || q.Get("per_page") != "100" {
			t.Errorf("query = %v", q)
		}
		switch q.Get("page") {
		case "":
			w.Header().Set("Link", fmt.Sprintf(`<%s/user/repos?sort=pushed&per_page=100&page=2>; rel="next"`, srv.URL))
			_, _ = w.Write(fixture(t, "repos_page1.json"))
		case "2":
			w.Header().Set("Link", fmt.Sprintf(`<%s/user/repos?sort=pushed&per_page=100&page=3>; rel="next"`, srv.URL))
			_, _ = w.Write(fixture(t, "repos_page2.json"))
		default:
			t.Errorf("fetched page %s past the cutoff", q.Get("page"))
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	c := New("tok", WithBaseURL(srv.URL))

	cutoff := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	repos, err := c.ListRepos(context.Background(), cutoff)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range repos {
		got = append(got, r.Key())
	}
	want := []string{"acme/api", "acme/web", "me/dotfiles"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("repos = %v, want %v", got, want)
	}
	if pages != 2 {
		t.Fatalf("fetched %d pages, want 2", pages)
	}
	if !repos[0].PushedAt.Equal(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("PushedAt = %v", repos[0].PushedAt)
	}
}

func TestListRunsDecodesAndForwardsParams(t *testing.T) {
	c := serveFixtures(t, map[string]string{"/repos/cli/cli/actions/runs": "runs.json"}, func(r *http.Request) {
		q := r.URL.Query()
		if q.Get("branch") != "trunk" || q.Get("status") != "completed" || q.Get("per_page") != "3" {
			t.Errorf("query = %v", q)
		}
		if r.Header.Get("If-None-Match") != `"etag1"` {
			t.Errorf("If-None-Match = %q", r.Header.Get("If-None-Match"))
		}
	})
	runs, _, err := c.ListRuns(context.Background(), "cli", "cli", RunsOpts{Branch: "trunk", Status: "completed", PerPage: 3}, `"etag1"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("len(runs) = %d", len(runs))
	}
	r := runs[0]
	if r.ID == 0 || r.Number == 0 || r.WorkflowName == "" || r.WorkflowID == 0 || r.Branch != "trunk" ||
		r.Event == "" || r.Actor == "" || r.Status == "" || r.CreatedAt.IsZero() || r.HTMLURL == "" || r.RepoKey != "cli/cli" {
		t.Fatalf("run not fully decoded: %+v", r)
	}
}

func TestListRunsNotModified(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	})
	runs, resp, err := c.ListRuns(context.Background(), "o", "r", RunsOpts{}, `"e"`)
	if err != nil || !resp.NotModified || runs != nil {
		t.Fatalf("runs=%v resp=%+v err=%v", runs, resp, err)
	}
}

func TestGetRun(t *testing.T) {
	c := serveFixtures(t, map[string]string{"/repos/cli/cli/actions/runs/37571611570": "run.json"}, nil)
	r, err := c.GetRun(context.Background(), "cli", "cli", 37571611570)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != 37571611570 || r.Status == "" || r.RepoKey != "cli/cli" {
		t.Fatalf("run = %+v", r)
	}
}

func TestListJobs(t *testing.T) {
	c := serveFixtures(t, map[string]string{"/repos/cli/cli/actions/runs/37571611570/jobs": "jobs.json"}, nil)
	jobs, err := c.ListJobs(context.Background(), "cli", "cli", 37571611570)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d", len(jobs))
	}
	j := jobs[0]
	if j.ID != 112631290209 || j.RunID != 37571611570 || j.Name == "" || j.Status != "completed" || j.Conclusion != "success" || j.StartedAt.IsZero() {
		t.Fatalf("job = %+v", j)
	}
	if len(j.Steps) == 0 || j.Steps[0].Number != 1 || j.Steps[0].Name != "Set up job" {
		t.Fatalf("steps = %+v", j.Steps)
	}
}

func TestGetJob(t *testing.T) {
	c := serveFixtures(t, map[string]string{"/repos/cli/cli/actions/jobs/112631290209": "job.json"}, nil)
	j, err := c.GetJob(context.Background(), "cli", "cli", 112631290209)
	if err != nil {
		t.Fatal(err)
	}
	if j.ID != 112631290209 || len(j.Steps) == 0 {
		t.Fatalf("job = %+v", j)
	}
}

func TestGetJobNotFound(t *testing.T) {
	c := serveFixtures(t, nil, nil)
	_, err := c.GetJob(context.Background(), "cli", "cli", 1)
	if !IsNotFound(err) {
		t.Fatalf("err = %v, want 404 APIError", err)
	}
}

func TestGetRepo(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/api" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"name":"api","owner":{"login":"acme"},"pushed_at":"2026-01-02T03:04:05Z","archived":true}`))
	})
	r, err := c.GetRepo(context.Background(), "acme", "api")
	if err != nil {
		t.Fatal(err)
	}
	if r.Key() != "acme/api" || !r.Archived || !r.PushedAt.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("repo = %+v", r)
	}
}

func TestRunDecodesCommit(t *testing.T) {
	var r Run
	err := json.Unmarshal([]byte(`{"id":1,"head_sha":"e2c23169990b8371552642850ff1daba1408ef01",
		"head_commit":{"id":"e2c23169990b8371552642850ff1daba1408ef01","message":"Fix the thing\n\nLonger body","author":{"name":"Ada"}},
		"repository":{"name":"r","owner":{"login":"o"}}}`), &r)
	if err != nil {
		t.Fatal(err)
	}
	if r.HeadSHA != "e2c23169990b8371552642850ff1daba1408ef01" || r.CommitMessage != "Fix the thing" || r.CommitAuthor != "Ada" {
		t.Fatalf("run = %+v", r)
	}
	if r.CommitURL() != "https://github.com/o/r/commit/e2c23169990b8371552642850ff1daba1408ef01" || r.ShortSHA() != "e2c2316" {
		t.Fatalf("url %q short %q", r.CommitURL(), r.ShortSHA())
	}
	var bare Run
	_ = json.Unmarshal([]byte(`{"id":2,"head_commit":null}`), &bare)
	if bare.CommitURL() != "" || bare.ShortSHA() != "" {
		t.Fatalf("run without a commit: url %q short %q", bare.CommitURL(), bare.ShortSHA())
	}
}
