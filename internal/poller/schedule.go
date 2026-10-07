package poller

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// schedule maps resource keys to the time they are next due. Keys are
// "repos", "runs:<owner/repo>", "jobs:<runID>" and "log:<jobID>".
type schedule struct {
	mu  sync.Mutex
	due map[string]time.Time
}

func newSchedule() *schedule { return &schedule{due: map[string]time.Time{}} }

func (s *schedule) set(key string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.due[key] = at
}

// setIfAbsent schedules key only if it is not scheduled yet.
func (s *schedule) setIfAbsent(key string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.due[key]; !ok {
		s.due[key] = at
	}
}

func (s *schedule) remove(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.due, key)
}

// removePrefix drops every key with prefix for which keep returns false.
func (s *schedule) removePrefix(prefix string, keep func(key string) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.due {
		if strings.HasPrefix(k, prefix) && !keep(k) {
			delete(s.due, k)
		}
	}
}

// dueKeys returns keys due at now: repos first, then jobs and logs (what
// the user is looking at), then runs, each group in key order.
func (s *schedule) dueKeys(now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for k, at := range s.due {
		if !at.After(now) {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		ri, rj := rank(keys[i]), rank(keys[j])
		if ri != rj {
			return ri < rj
		}
		return keys[i] < keys[j]
	})
	return keys
}

func rank(key string) int {
	for i, p := range []string{"repos", "jobs:", "log:", "runs:"} {
		if strings.HasPrefix(key, p) {
			return i
		}
	}
	return 9
}
