package store

import (
	"slices"

	"github.com/rnjn/ghat/internal/tail"
)

// maxLogs bounds how many job logs are kept in memory.
const maxLogs = 5

// LogBuffer is the parsed log of one job. Lines only ever grow and existing
// lines never change, so a LogBuffer returned by Log is a stable snapshot.
type LogBuffer struct {
	JobID    int64
	Lines    []tail.LogLine
	Complete bool
}

// SetLog stores the full parsed log of a job, keeping lines already held
// and appending the rest. It returns the appended range [from, to).
func (s *Store) SetLog(jobID int64, lines []tail.LogLine, complete bool) (from, to int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.logs[jobID]
	if !ok {
		b = &LogBuffer{JobID: jobID}
		s.logs[jobID] = b
	}
	s.touch(jobID)
	from = len(b.Lines)
	if len(lines) > from {
		b.Lines = append(b.Lines, lines[from:]...)
	}
	b.Complete = b.Complete || complete
	return from, len(b.Lines)
}

// touch marks jobID most recently used and evicts beyond maxLogs.
func (s *Store) touch(jobID int64) {
	if i := slices.Index(s.logOrder, jobID); i >= 0 {
		s.logOrder = slices.Delete(s.logOrder, i, i+1)
	}
	s.logOrder = append(s.logOrder, jobID)
	for len(s.logOrder) > maxLogs {
		delete(s.logs, s.logOrder[0])
		s.logOrder = s.logOrder[1:]
	}
}

// Log returns a snapshot of a job's log without copying the lines.
func (s *Store) Log(jobID int64) LogBuffer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.logs[jobID]
	if !ok {
		return LogBuffer{JobID: jobID}
	}
	n := len(b.Lines)
	return LogBuffer{JobID: jobID, Lines: b.Lines[:n:n], Complete: b.Complete}
}

// DropLog forgets a job's log.
func (s *Store) DropLog(jobID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.logs, jobID)
	if i := slices.Index(s.logOrder, jobID); i >= 0 {
		s.logOrder = slices.Delete(s.logOrder, i, i+1)
	}
}
