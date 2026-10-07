// Package cache persists the last discovery result, runs and ETags so a
// restart shows the board at once and its first polls are cheap 304s. Any
// problem reading the cache is treated as a miss.
package cache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/rnjn/ghtui/internal/gh"
	"github.com/rnjn/ghtui/internal/store"
)

// Version changes whenever the file format does; other versions are misses.
const Version = 1

const fileName = "state.json"

// Snapshot is what the cache holds.
type Snapshot struct {
	Version int                 `json:"version"`
	SavedAt time.Time           `json:"saved_at"`
	Repos   []repoRecord        `json:"repos"`
	Runs    map[string][]runRec `json:"runs"` // only repos polled at least once
}

// repoRecord and runRec are the cache's own shapes: gh.Repo and gh.Run
// decode GitHub's API format, which differs.
type repoRecord struct {
	Owner, Name, DefaultBranch string
	PushedAt                   time.Time
	Archived, Pinned           bool
	RunsETag                   string
}

type runRec struct {
	ID, WorkflowID                                      int64
	Number                                              int
	RepoKey, WorkflowName, Branch, Event, Actor, Status string
	Conclusion, HTMLURL                                 string
	CreatedAt, UpdatedAt                                time.Time
}

// Dir is $XDG_CACHE_HOME/ghtui, else ~/.cache/ghtui.
func Dir() string {
	if x := os.Getenv("XDG_CACHE_HOME"); x != "" {
		return filepath.Join(x, "ghtui")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "ghtui")
}

// FromStore captures the store's repos and runs.
func FromStore(st *store.Store, now time.Time) Snapshot {
	s := Snapshot{Version: Version, SavedAt: now, Runs: map[string][]runRec{}}
	for _, r := range st.Repos() {
		s.Repos = append(s.Repos, repoRecord{
			Owner: r.Repo.Owner, Name: r.Repo.Name, DefaultBranch: r.Repo.DefaultBranch,
			PushedAt: r.Repo.PushedAt, Archived: r.Repo.Archived, Pinned: r.Pinned, RunsETag: r.RunsETag,
		})
		if !r.Polled {
			continue
		}
		recs := []runRec{}
		for _, run := range st.Runs(r.Repo.Key()) {
			recs = append(recs, runRec{
				ID: run.ID, WorkflowID: run.WorkflowID, Number: run.Number, RepoKey: run.RepoKey,
				WorkflowName: run.WorkflowName, Branch: run.Branch, Event: run.Event, Actor: run.Actor,
				Status: run.Status, Conclusion: run.Conclusion, HTMLURL: run.HTMLURL,
				CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
			})
		}
		s.Runs[r.Repo.Key()] = recs
	}
	return s
}

// Apply loads the snapshot into an empty store.
func (s Snapshot) Apply(st *store.Store) {
	repos := make([]store.RepoState, 0, len(s.Repos))
	for _, r := range s.Repos {
		repos = append(repos, store.RepoState{
			Repo:   gh.Repo{Owner: r.Owner, Name: r.Name, PushedAt: r.PushedAt, Archived: r.Archived, DefaultBranch: r.DefaultBranch},
			Pinned: r.Pinned, RunsETag: r.RunsETag,
		})
	}
	st.SetRepos(repos)
	for _, r := range repos {
		recs, ok := s.Runs[r.Repo.Key()]
		if !ok {
			continue
		}
		runs := make([]gh.Run, len(recs))
		for i, x := range recs {
			runs[i] = gh.Run{
				ID: x.ID, WorkflowID: x.WorkflowID, Number: x.Number, RepoKey: x.RepoKey,
				WorkflowName: x.WorkflowName, Branch: x.Branch, Event: x.Event, Actor: x.Actor,
				Status: x.Status, Conclusion: x.Conclusion, HTMLURL: x.HTMLURL,
				CreatedAt: x.CreatedAt, UpdatedAt: x.UpdatedAt,
			}
		}
		st.SetRuns(r.Repo.Key(), runs, r.RunsETag)
	}
}

// Load reads the cache; ok is false on any problem.
func Load(dir string) (s Snapshot, ok bool) {
	b, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		return Snapshot{}, false
	}
	if err := json.Unmarshal(b, &s); err != nil || s.Version != Version {
		return Snapshot{}, false
	}
	return s, true
}

// Save writes the cache atomically: a temp file in dir renamed over the
// old one, so readers never see a partial file.
func Save(dir string, s Snapshot) (err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, fileName+".*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(f.Name())
		}
	}()
	if _, err = f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, fileName)); err != nil {
		return errors.Join(errors.New("save cache"), err)
	}
	return nil
}
