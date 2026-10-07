// Package store holds the Repo → Run → Job tree and log buffers the TUI
// reads. It is pure data: no I/O, safe for concurrent use.
package store

import (
	"slices"
	"sort"
	"sync"

	"ghtui/internal/gh"
)

// RepoState is a repo plus what the poller knows about it.
type RepoState struct {
	Repo        gh.Repo
	Pinned      bool
	Unavailable bool
	RunsETag    string
	LastError   string
}

// Store is the in-memory model shared by poller (writer) and TUI (reader).
type Store struct {
	mu      sync.RWMutex
	repos   map[string]RepoState
	runs    map[string][]gh.Run // by repo key, newest first
	jobs    map[int64][]gh.Job  // by run ID
	watched map[int64]bool
}

// New returns an empty store.
func New() *Store {
	return &Store{
		repos:   map[string]RepoState{},
		runs:    map[string][]gh.Run{},
		jobs:    map[int64][]gh.Job{},
		watched: map[int64]bool{},
	}
}

// SetRepos replaces the repo set. Repos still present keep their ETag and
// are made available again; removed repos lose their runs.
func (s *Store) SetRepos(repos []RepoState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]RepoState, len(repos))
	for _, r := range repos {
		k := r.Repo.Key()
		if old, ok := s.repos[k]; ok && r.RunsETag == "" {
			r.RunsETag = old.RunsETag
		}
		r.Unavailable, r.LastError = false, ""
		next[k] = r
	}
	for k, runs := range s.runs {
		if _, ok := next[k]; !ok {
			for _, r := range runs {
				delete(s.jobs, r.ID)
				delete(s.watched, r.ID)
			}
			delete(s.runs, k)
		}
	}
	s.repos = next
}

// Repos returns all repos, repos with active runs first, then most
// recently pushed.
func (s *Store) Repos() []RepoState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RepoState, 0, len(s.repos))
	for _, r := range s.repos {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		ai, aj := s.anyActive(out[i].Repo.Key()), s.anyActive(out[j].Repo.Key())
		if ai != aj {
			return ai
		}
		if !out[i].Repo.PushedAt.Equal(out[j].Repo.PushedAt) {
			return out[i].Repo.PushedAt.After(out[j].Repo.PushedAt)
		}
		return out[i].Repo.Key() < out[j].Repo.Key()
	})
	return out
}

// Repo returns one repo's state.
func (s *Store) Repo(key string) (RepoState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.repos[key]
	return r, ok
}

// MarkUnavailable flags a repo as unreadable until the next SetRepos.
func (s *Store) MarkUnavailable(repoKey, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.repos[repoKey]; ok {
		r.Unavailable, r.LastError = true, reason
		s.repos[repoKey] = r
	}
}

// SetRuns replaces a repo's runs and ETag. It returns watched runs that
// just moved to completed; those are unwatched.
func (s *Store) SetRuns(repoKey string, runs []gh.Run, etag string) (completedWatched []gh.Run) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := map[int64]string{}
	for _, r := range s.runs[repoKey] {
		prev[r.ID] = r.Status
	}
	sorted := slices.Clone(runs)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CreatedAt.After(sorted[j].CreatedAt) })
	for _, r := range sorted {
		if s.watched[r.ID] && r.Status == "completed" && prev[r.ID] != "completed" {
			completedWatched = append(completedWatched, r)
			delete(s.watched, r.ID)
		}
	}
	s.runs[repoKey] = sorted
	if rs, ok := s.repos[repoKey]; ok {
		rs.RunsETag = etag
		s.repos[repoKey] = rs
	}
	return completedWatched
}

// Runs returns a repo's runs, newest first.
func (s *Store) Runs(repoKey string) []gh.Run {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.runs[repoKey])
}

// Run finds a run by ID in any repo.
func (s *Store) Run(id int64) (gh.Run, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.run(id)
}

func (s *Store) run(id int64) (gh.Run, bool) {
	for _, runs := range s.runs {
		for _, r := range runs {
			if r.ID == id {
				return r, true
			}
		}
	}
	return gh.Run{}, false
}

// SetJobs replaces a run's jobs.
func (s *Store) SetJobs(runID int64, jobs []gh.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[runID] = cloneJobs(jobs)
}

// Jobs returns a run's jobs.
func (s *Store) Jobs(runID int64) []gh.Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneJobs(s.jobs[runID])
}

func cloneJobs(jobs []gh.Job) []gh.Job {
	if jobs == nil {
		return nil
	}
	out := make([]gh.Job, len(jobs))
	for i, j := range jobs {
		j.Steps = slices.Clone(j.Steps)
		out[i] = j
	}
	return out
}

// ToggleWatch flips the watch flag of a known run and returns the new value.
func (s *Store) ToggleWatch(runID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.run(runID); !ok {
		return false
	}
	if s.watched[runID] {
		delete(s.watched, runID)
		return false
	}
	s.watched[runID] = true
	return true
}

// Watched reports whether a run is watched.
func (s *Store) Watched(runID int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.watched[runID]
}

// WatchedRuns returns watched run IDs in ascending order.
func (s *Store) WatchedRuns() []int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]int64, 0, len(s.watched))
	for id := range s.watched {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// activeStatuses are run statuses that mean work is pending or running.
var activeStatuses = map[string]bool{"queued": true, "in_progress": true, "waiting": true, "requested": true, "pending": true}

// IsActive reports whether a run or job status means not yet finished.
func IsActive(status string) bool { return activeStatuses[status] }

// AnyActive reports whether a repo has a run that has not finished.
func (s *Store) AnyActive(repoKey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.anyActive(repoKey)
}

func (s *Store) anyActive(repoKey string) bool {
	for _, r := range s.runs[repoKey] {
		if activeStatuses[r.Status] {
			return true
		}
	}
	return false
}
