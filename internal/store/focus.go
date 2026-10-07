package store

type tailTarget struct {
	owner, repo string
	jobID       int64
}

// SetFocusRun records the run the user is looking at; 0 clears it. Jobs are
// polled only for the focused run and watched runs.
func (s *Store) SetFocusRun(runID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.focusRun = runID
}

// FocusRun returns the focused run ID, or 0.
func (s *Store) FocusRun() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.focusRun
}

// SetTailJob records the job open in the Tail view; jobID 0 clears it.
func (s *Store) SetTailJob(owner, repo string, jobID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tail = tailTarget{owner: owner, repo: repo, jobID: jobID}
}

// TailJob returns the job open in the Tail view; jobID is 0 if none.
func (s *Store) TailJob() (owner, repo string, jobID int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tail.owner, s.tail.repo, s.tail.jobID
}
