package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// fakeGH scripts a job whose status and log advance one step per poll.
type fakeGH struct {
	mu       sync.Mutex
	jobID    int64
	statuses []string                   // per GetJob call; last repeats
	conc     string                     // conclusion once completed
	logs     []string                   // per log call; last repeats; "" = 404
	runJobs  map[int64][]map[string]any // fixed job lists by run ID
	jobsSeq  [][]map[string]any         // per ListJobs call for other runs; last repeats
	runs     []map[string]any           // per GetRun call; last repeats
	jobCalls int
	logCalls int
	runCalls int
	listJobs int
}

func pick[T any](xs []T, i int) T { return xs[min(i, len(xs)-1)] }

func (f *fakeGH) jobJSON(status string) map[string]any {
	j := map[string]any{"id": f.jobID, "run_id": 9, "name": "test", "status": status}
	if status == "completed" {
		j["conclusion"] = f.conc
	}
	return j
}

func (f *fakeGH) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		p := strings.TrimPrefix(r.URL.Path, "/repos/acme/api/actions/")
		jobPath := fmt.Sprintf("jobs/%d", f.jobID)
		switch {
		case p == jobPath && f.statuses != nil:
			s := pick(f.statuses, f.jobCalls)
			f.jobCalls++
			_ = json.NewEncoder(w).Encode(f.jobJSON(s))
		case p == jobPath+"/logs" && f.logs != nil:
			body := pick(f.logs, f.logCalls)
			f.logCalls++
			if body == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(body))
		case strings.HasPrefix(p, "runs/") && strings.HasSuffix(p, "/jobs"):
			var id int64
			_, _ = fmt.Sscanf(p, "runs/%d/jobs", &id)
			jobs, ok := f.runJobs[id]
			if !ok && f.jobsSeq == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if !ok {
				jobs = pick(f.jobsSeq, f.listJobs)
			}
			f.listJobs++
			_ = json.NewEncoder(w).Encode(map[string]any{"total_count": len(jobs), "jobs": jobs})
		case strings.HasPrefix(p, "runs/") && f.runs != nil:
			run := pick(f.runs, f.runCalls)
			f.runCalls++
			_ = json.NewEncoder(w).Encode(run)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		}
	}
}
