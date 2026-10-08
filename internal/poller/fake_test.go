package poller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rnjn/ghat/internal/config"
	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/store"
)

type logResp struct {
	body string
	err  error
}

// fakeAPI serves scripted data and records every call.
type fakeAPI struct {
	mu        sync.Mutex
	calls     []string
	repos     []gh.Repo
	reposErr  error
	since     time.Time
	getRepo   map[string]gh.Repo
	runs      map[string][]gh.Run
	runsETag  map[string]string
	runsErr   map[string]error
	jobs      map[int64][]gh.Job
	jobsErr   map[int64]error
	job       map[int64]gh.Job
	logs      map[int64][]logResp // per call; last repeats
	logCalls  map[int64]int
	remaining int
	reset     time.Time
	onRuns    func()      // called (unlocked) after each ListRuns
	runsOpts  gh.RunsOpts // options of the latest ListRuns
}

func newFake() *fakeAPI {
	return &fakeAPI{
		getRepo: map[string]gh.Repo{}, runs: map[string][]gh.Run{}, runsETag: map[string]string{},
		runsErr: map[string]error{}, jobs: map[int64][]gh.Job{}, jobsErr: map[int64]error{},
		job: map[int64]gh.Job{}, logs: map[int64][]logResp{}, logCalls: map[int64]int{}, remaining: 5000,
	}
}

func (f *fakeAPI) record(format string, args ...any) {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

func (f *fakeAPI) takeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.calls
	f.calls = nil
	return c
}

func (f *fakeAPI) ListRepos(_ context.Context, since time.Time) ([]gh.Repo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("ListRepos")
	f.since = since
	return f.repos, f.reposErr
}

func (f *fakeAPI) GetRepo(_ context.Context, owner, repo string) (gh.Repo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("GetRepo %s/%s", owner, repo)
	r, ok := f.getRepo[owner+"/"+repo]
	if !ok {
		return gh.Repo{}, &gh.APIError{Status: 404}
	}
	return r, nil
}

func (f *fakeAPI) ListRuns(_ context.Context, owner, repo string, opts gh.RunsOpts, etag string) ([]gh.Run, gh.Response, error) {
	if f.onRuns != nil {
		defer f.onRuns()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	k := owner + "/" + repo
	f.runsOpts = opts
	f.record("ListRuns %s etag=%s", k, etag)
	if err := f.runsErr[k]; err != nil {
		return nil, gh.Response{}, err
	}
	if e := f.runsETag[k]; e != "" && e == etag {
		return nil, gh.Response{ETag: e, NotModified: true}, nil
	}
	return f.runs[k], gh.Response{ETag: f.runsETag[k]}, nil
}

func (f *fakeAPI) ListJobs(_ context.Context, owner, repo string, runID int64) ([]gh.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("ListJobs %d", runID)
	return f.jobs[runID], f.jobsErr[runID]
}

func (f *fakeAPI) GetJob(_ context.Context, owner, repo string, id int64) (gh.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("GetJob %d", id)
	j, ok := f.job[id]
	if !ok {
		return gh.Job{}, &gh.APIError{Status: 404}
	}
	return j, nil
}

func (f *fakeAPI) JobLog(_ context.Context, owner, repo string, jobID int64) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("JobLog %d", jobID)
	seq := f.logs[jobID]
	if len(seq) == 0 {
		return nil, gh.ErrLogNotReady
	}
	r := seq[min(f.logCalls[jobID], len(seq)-1)]
	f.logCalls[jobID]++
	return []byte(r.body), r.err
}

func (f *fakeAPI) RateLimit() (int, time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.remaining, f.reset
}

// harness wires a fake API, store and poller with a message recorder.
type harness struct {
	api  *fakeAPI
	st   *store.Store
	p    *Poller
	msgs []any
	now  time.Time
}

func newHarness(repos ...gh.Repo) *harness {
	h := &harness{api: newFake(), st: store.New(), now: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)}
	h.api.repos = repos
	h.p = New(h.api, h.st, config.Default(), func(m any) { h.msgs = append(h.msgs, m) })
	return h
}

func (h *harness) tickAt(d time.Duration) {
	h.p.Tick(context.Background(), h.now.Add(d))
}

func (h *harness) takeMsgs() []any {
	m := h.msgs
	h.msgs = nil
	return m
}

func ghRepo(owner, name string, pushedAgo time.Duration) gh.Repo {
	return gh.Repo{Owner: owner, Name: name, PushedAt: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC).Add(-pushedAgo)}
}

func ghRun(id int64, repoKey, status, conclusion string) gh.Run {
	return gh.Run{ID: id, RepoKey: repoKey, Status: status, Conclusion: conclusion,
		CreatedAt: time.Date(2026, 10, 7, 11, 0, 0, 0, time.UTC).Add(time.Duration(id) * time.Second)}
}

func countCalls(calls []string, prefix string) int {
	n := 0
	for _, c := range calls {
		if len(c) >= len(prefix) && c[:len(prefix)] == prefix {
			n++
		}
	}
	return n
}
