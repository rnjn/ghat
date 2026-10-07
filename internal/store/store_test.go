package store

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rnjn/ghtui/internal/gh"
)

func t0() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC) }

func repo(owner, name string, pushedAgo time.Duration) RepoState {
	return RepoState{Repo: gh.Repo{Owner: owner, Name: name, PushedAt: t0().Add(-pushedAgo)}}
}

func run(id int64, repoKey, status, conclusion string) gh.Run {
	return gh.Run{ID: id, RepoKey: repoKey, Status: status, Conclusion: conclusion, CreatedAt: t0().Add(time.Duration(id) * time.Minute)}
}

func keys(rs []RepoState) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Repo.Key())
	}
	return fmt.Sprint(out)
}

func TestReposSortedActiveFirstThenPushed(t *testing.T) {
	s := New()
	s.SetRepos([]RepoState{repo("a", "old", 48*time.Hour), repo("a", "new", time.Hour), repo("a", "busy", 72*time.Hour)})
	s.SetRuns("a/busy", []gh.Run{run(1, "a/busy", "in_progress", "")}, "")
	if got := keys(s.Repos()); got != "[a/busy a/new a/old]" {
		t.Fatalf("order = %s", got)
	}
}

func TestSetReposPreservesETagAndDropsRemoved(t *testing.T) {
	s := New()
	s.SetRepos([]RepoState{repo("a", "x", 0), repo("a", "y", 0)})
	s.SetRuns("a/x", []gh.Run{run(1, "a/x", "completed", "success")}, `"e1"`)
	s.SetRuns("a/y", []gh.Run{run(2, "a/y", "completed", "success")}, `"e2"`)
	s.MarkUnavailable("a/x", "404")
	s.SetRepos([]RepoState{repo("a", "x", 0)})
	rs := s.Repos()
	if len(rs) != 1 || rs[0].RunsETag != `"e1"` || rs[0].Unavailable {
		t.Fatalf("repos = %+v", rs)
	}
	if len(s.Runs("a/y")) != 0 {
		t.Fatal("runs of removed repo kept")
	}
	if _, ok := s.Run(2); ok {
		t.Fatal("Run(2) still found")
	}
}

func TestGettersReturnCopies(t *testing.T) {
	s := New()
	s.SetRepos([]RepoState{repo("a", "x", 0)})
	s.SetRuns("a/x", []gh.Run{run(1, "a/x", "queued", "")}, "")
	s.SetJobs(1, []gh.Job{{ID: 5, Steps: []gh.Step{{Number: 1, Name: "s"}}}})
	s.Runs("a/x")[0].Status = "mutated"
	s.Jobs(1)[0].Steps[0].Name = "mutated"
	s.Repos()[0].RunsETag = "mutated"
	if s.Runs("a/x")[0].Status != "queued" || s.Jobs(1)[0].Steps[0].Name != "s" || s.Repos()[0].RunsETag != "" {
		t.Fatal("store mutated through a getter result")
	}
}

func TestRunsNewestFirstAndLookup(t *testing.T) {
	s := New()
	s.SetRepos([]RepoState{repo("a", "x", 0)})
	s.SetRuns("a/x", []gh.Run{run(1, "a/x", "queued", ""), run(3, "a/x", "queued", ""), run(2, "a/x", "queued", "")}, "")
	rs := s.Runs("a/x")
	if rs[0].ID != 3 || rs[2].ID != 1 {
		t.Fatalf("order = %v", rs)
	}
	if r, ok := s.Run(2); !ok || r.ID != 2 {
		t.Fatal("Run(2) not found")
	}
}

func TestWatchedRunCompletionReportedOnce(t *testing.T) {
	s := New()
	s.SetRepos([]RepoState{repo("a", "x", 0)})
	s.SetRuns("a/x", []gh.Run{run(1, "a/x", "in_progress", ""), run(2, "a/x", "in_progress", "")}, "")
	if !s.ToggleWatch(1) || !s.Watched(1) {
		t.Fatal("watch not set")
	}
	got := s.SetRuns("a/x", []gh.Run{run(1, "a/x", "completed", "failure"), run(2, "a/x", "completed", "success")}, "")
	if len(got) != 1 || got[0].ID != 1 || got[0].Conclusion != "failure" {
		t.Fatalf("completed watched = %+v", got)
	}
	if s.Watched(1) || len(s.WatchedRuns()) != 0 {
		t.Fatal("completed run still watched")
	}
	if again := s.SetRuns("a/x", []gh.Run{run(1, "a/x", "completed", "failure")}, ""); len(again) != 0 {
		t.Fatalf("reported twice: %+v", again)
	}
}

func TestToggleWatchUnknownRun(t *testing.T) {
	s := New()
	if s.ToggleWatch(99) {
		t.Fatal("watched a run the store does not know")
	}
}

func TestAnyActive(t *testing.T) {
	s := New()
	s.SetRepos([]RepoState{repo("a", "x", 0)})
	for _, st := range []string{"queued", "in_progress", "waiting", "requested", "pending"} {
		s.SetRuns("a/x", []gh.Run{run(1, "a/x", st, "")}, "")
		if !s.AnyActive("a/x") {
			t.Errorf("%s not active", st)
		}
	}
	s.SetRuns("a/x", []gh.Run{run(1, "a/x", "completed", "success")}, "")
	if s.AnyActive("a/x") {
		t.Error("completed counted active")
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New()
	s.SetRepos([]RepoState{repo("a", "x", 0)})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			s.SetRuns("a/x", []gh.Run{run(int64(i), "a/x", "queued", "")}, "")
			s.SetJobs(int64(i), []gh.Job{{ID: int64(i)}})
			s.ToggleWatch(int64(i))
		}(i)
		go func() {
			defer wg.Done()
			_ = s.Repos()
			_ = s.Runs("a/x")
			_ = s.WatchedRuns()
		}()
	}
	wg.Wait()
}

func TestPolledSetBySetRunsAndKeptAcrossRediscovery(t *testing.T) {
	s := New()
	s.SetRepos([]RepoState{repo("a", "x", 0)})
	if r, _ := s.Repo("a/x"); r.Polled {
		t.Fatal("polled before any runs poll")
	}
	s.SetRuns("a/x", nil, "")
	if r, _ := s.Repo("a/x"); !r.Polled {
		t.Fatal("SetRuns did not mark polled")
	}
	s.SetRepos([]RepoState{repo("a", "x", 0)})
	if r, _ := s.Repo("a/x"); !r.Polled {
		t.Fatal("rediscovery lost the polled flag")
	}
}
