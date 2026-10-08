package tail

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rnjn/ghat/internal/gh"
)

// fakeAPI serves jobs by ID and runs' job lists by run ID.
func fakeAPI(t *testing.T, jobs map[string]gh.Job, runs map[string][]gh.Job) *gh.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/repos/o/r/actions/")
		if id, ok := strings.CutPrefix(p, "jobs/"); ok {
			if j, ok := jobs[id]; ok {
				_ = json.NewEncoder(w).Encode(j)
				return
			}
		}
		if rest, ok := strings.CutPrefix(p, "runs/"); ok {
			if js, ok := runs[strings.TrimSuffix(rest, "/jobs")]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"total_count": len(js), "jobs": js})
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	t.Cleanup(srv.Close)
	return gh.New("tok", gh.WithBaseURL(srv.URL))
}

func job(id int64, name, status string) gh.Job { return gh.Job{ID: id, Name: name, Status: status} }

func TestResolveJobByJobID(t *testing.T) {
	c := fakeAPI(t, map[string]gh.Job{"5": job(5, "build", "in_progress")}, nil)
	j, err := ResolveJob(context.Background(), c, "o", "r", 5)
	if err != nil || j.ID != 5 {
		t.Fatalf("j=%+v err=%v", j, err)
	}
}

func TestResolveJobRunWithSingleJob(t *testing.T) {
	c := fakeAPI(t, nil, map[string][]gh.Job{"9": {job(11, "build", "completed")}})
	j, err := ResolveJob(context.Background(), c, "o", "r", 9)
	if err != nil || j.ID != 11 {
		t.Fatalf("j=%+v err=%v", j, err)
	}
}

func TestResolveJobRunWithOneInProgress(t *testing.T) {
	c := fakeAPI(t, nil, map[string][]gh.Job{"9": {
		job(11, "lint", "completed"), job(12, "test", "in_progress"), job(13, "deploy", "queued"),
	}})
	j, err := ResolveJob(context.Background(), c, "o", "r", 9)
	if err != nil || j.ID != 12 {
		t.Fatalf("j=%+v err=%v", j, err)
	}
}

func TestResolveJobAmbiguousRun(t *testing.T) {
	for name, jobs := range map[string][]gh.Job{
		"several in progress": {job(11, "lint", "in_progress"), job(12, "test", "in_progress")},
		"none in progress":    {job(11, "lint", "completed"), job(12, "test", "completed")},
	} {
		t.Run(name, func(t *testing.T) {
			c := fakeAPI(t, nil, map[string][]gh.Job{"9": jobs})
			_, err := ResolveJob(context.Background(), c, "o", "r", 9)
			if !errors.Is(err, ErrAmbiguousRun) {
				t.Fatalf("err = %v, want ErrAmbiguousRun", err)
			}
			var ae *AmbiguousRunError
			if !errors.As(err, &ae) || len(ae.Jobs) != 2 {
				t.Fatalf("err = %#v", err)
			}
			msg := err.Error()
			for _, want := range []string{"11", "lint", "12", "test"} {
				if !strings.Contains(msg, want) {
					t.Errorf("message %q lacks %q", msg, want)
				}
			}
		})
	}
}

func TestResolveJobNotFound(t *testing.T) {
	c := fakeAPI(t, nil, nil)
	_, err := ResolveJob(context.Background(), c, "o", "r", 404)
	if !gh.IsNotFound(err) || !strings.Contains(err.Error(), "no job or run 404 in o/r") {
		t.Fatalf("err = %v, want 404 naming the ID and repo", err)
	}
}

func TestResolveJobRunWithoutJobs(t *testing.T) {
	c := fakeAPI(t, nil, map[string][]gh.Job{"9": {}})
	_, err := ResolveJob(context.Background(), c, "o", "r", 9)
	if err == nil || !strings.Contains(err.Error(), "no jobs") {
		t.Fatalf("err = %v", err)
	}
}
