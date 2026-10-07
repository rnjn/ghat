package poller

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"ghtui/internal/config"
	"ghtui/internal/gh"
	"ghtui/internal/store"
)

// API is the subset of *gh.Client the poller uses.
type API interface {
	ListRepos(ctx context.Context, pushedSince time.Time) ([]gh.Repo, error)
	GetRepo(ctx context.Context, owner, repo string) (gh.Repo, error)
	ListRuns(ctx context.Context, owner, repo string, opts gh.RunsOpts, etag string) ([]gh.Run, gh.Response, error)
	ListJobs(ctx context.Context, owner, repo string, runID int64) ([]gh.Job, error)
	GetJob(ctx context.Context, owner, repo string, id int64) (gh.Job, error)
	JobLog(ctx context.Context, owner, repo string, jobID int64) ([]byte, error)
	RateLimit() (remaining int, reset time.Time)
}

const (
	discoveryInterval = 10 * time.Minute
	runsPerPage       = 30
)

// Poller schedules GitHub calls. Tick does all due work synchronously, so
// tests drive it with explicit times; Run drives it with a real ticker.
type Poller struct {
	api   API
	st    *store.Store
	cfg   config.Config
	send  func(any)
	sched *schedule
	wake  chan struct{}

	// Poller-goroutine-only state.
	polled     map[string]bool // runs keys polled at least once
	jobsDone   map[string]bool // jobs keys of completed runs already fetched
	tailJob    int64
	logRetries int
	logDone    bool
	failures   map[string]int
	authFailed bool
	pauseUntil time.Time
	lastRate   RateLimit

	afterDiscovery func()
}

// Option configures a Poller.
type Option func(*Poller)

// WithAfterDiscovery runs f (on the poller goroutine) after each successful
// discovery; used to save the cache.
func WithAfterDiscovery(f func()) Option {
	return func(p *Poller) { p.afterDiscovery = f }
}

// New returns a poller writing into st and reporting changes through send.
func New(api API, st *store.Store, cfg config.Config, send func(any), opts ...Option) *Poller {
	p := &Poller{api: api, st: st, cfg: cfg, send: send, sched: newSchedule(), wake: make(chan struct{}, 1), polled: map[string]bool{}, jobsDone: map[string]bool{}, failures: map[string]int{}}
	for _, o := range opts {
		o(p)
	}
	p.sched.set("repos", time.Time{})
	return p
}

// Refresh makes a resource due immediately ("repos", "runs:<owner/repo>",
// "jobs:<runID>", "log:<jobID>").
func (p *Poller) Refresh(resource string) {
	p.sched.set(resource, time.Time{})
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Run ticks once a second, and immediately on Refresh, until ctx is done.
func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		p.Tick(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-p.wake:
		}
	}
}

// Tick runs every resource poll that is due at now, including resources
// that become due during the tick (repos found by discovery), each once.
func (p *Poller) Tick(ctx context.Context, now time.Time) {
	defer p.reportRate()
	p.syncInterest()
	done := map[string]bool{}
	for {
		if p.paused(now) {
			return
		}
		var key string
		for _, k := range p.sched.dueKeys(now) {
			if !done[k] {
				key = k
				break
			}
		}
		if key == "" || ctx.Err() != nil {
			return
		}
		done[key] = true
		p.poll(ctx, now, key)
	}
}

func (p *Poller) poll(ctx context.Context, now time.Time, key string) {
	switch {
	case key == "repos":
		p.discover(ctx, now)
	case strings.HasPrefix(key, "runs:"):
		p.pollRuns(ctx, now, strings.TrimPrefix(key, "runs:"))
	case strings.HasPrefix(key, "jobs:"):
		p.pollJobs(ctx, now, key)
	case strings.HasPrefix(key, "log:"):
		p.pollLog(ctx, now, key)
	default:
		p.sched.remove(key)
	}
}

// discover rebuilds the repo set: pushed within the window, plus pinned,
// minus excluded. New repos get their first runs poll now, then staggered.
func (p *Poller) discover(ctx context.Context, now time.Time) {
	p.sched.set("repos", now.Add(p.scale(discoveryInterval)))
	found, err := p.api.ListRepos(ctx, now.Add(-p.cfg.Repos.PushedWithin.D()))
	if err != nil {
		p.failed("repos", now, discoveryInterval, err)
		return
	}
	p.succeeded("repos")
	excluded := func(k string) bool { return slices.Contains(p.cfg.Repos.Exclude, k) }
	var states []store.RepoState
	seen := map[string]bool{}
	for _, r := range found {
		if excluded(r.Key()) {
			continue
		}
		seen[r.Key()] = true
		states = append(states, store.RepoState{Repo: r, Pinned: slices.Contains(p.cfg.Repos.Pinned, r.Key())})
	}
	var missing []string
	for _, k := range p.cfg.Repos.Pinned {
		if seen[k] || excluded(k) {
			continue
		}
		owner, name, err := config.ParseRepo(k)
		if err != nil {
			continue
		}
		r, err := p.api.GetRepo(ctx, owner, name)
		if err != nil {
			r = gh.Repo{Owner: owner, Name: name}
			missing = append(missing, k)
		}
		seen[k] = true
		states = append(states, store.RepoState{Repo: r, Pinned: true})
	}
	p.st.SetRepos(states)
	for _, k := range missing {
		p.st.MarkUnavailable(k, "repository not found")
	}
	p.sched.removePrefix("runs:", func(key string) bool { return seen[strings.TrimPrefix(key, "runs:")] })
	for k := range p.failures {
		if r, ok := strings.CutPrefix(k, "runs:"); ok && !seen[r] {
			delete(p.failures, k)
		}
	}
	for k := range p.polled {
		if !seen[k] {
			delete(p.polled, k)
		}
	}
	for _, s := range states {
		p.sched.setIfAbsent("runs:"+s.Repo.Key(), time.Time{})
	}
	p.send(ReposUpdated{})
	if p.afterDiscovery != nil {
		p.afterDiscovery()
	}
}

// pollRuns fetches one repo's runs with its ETag.
func (p *Poller) pollRuns(ctx context.Context, now time.Time, repoKey string) {
	first := !p.polled[repoKey]
	p.polled[repoKey] = true
	rs, ok := p.st.Repo(repoKey)
	if !ok {
		p.sched.remove("runs:" + repoKey)
		return
	}
	key := "runs:" + repoKey
	if rs.Unavailable {
		p.sched.remove(key)
		return
	}
	runs, resp, err := p.api.ListRuns(ctx, rs.Repo.Owner, rs.Repo.Name, gh.RunsOpts{PerPage: runsPerPage}, rs.RunsETag)
	var ae *gh.APIError
	switch {
	case errors.As(err, &ae) && (ae.Status == http.StatusNotFound || ae.Status == http.StatusForbidden):
		// Unreadable (no access, Actions disabled): skip until rediscovery.
		p.st.MarkUnavailable(repoKey, ae.Error())
		p.sched.remove(key)
		p.send(RunsUpdated{RepoKey: repoKey})
		return
	case err != nil:
		p.failed(key, now, p.runsInterval(repoKey), err)
		return
	}
	p.succeeded(key)
	p.send(Polled{})
	if !resp.NotModified {
		completed := p.st.SetRuns(repoKey, runs, resp.ETag)
		p.send(RunsUpdated{RepoKey: repoKey})
		for _, r := range completed {
			p.send(RunCompleted{Run: r})
		}
	}
	p.sched.set(key, p.nextRuns(now, repoKey, first))
}

func (p *Poller) runsInterval(repoKey string) time.Duration {
	if p.st.AnyActive(repoKey) {
		return p.cfg.Poll.RunsActive.D()
	}
	return p.cfg.Poll.RunsIdle.D()
}

// nextRuns is 15s for repos with active runs, else 60s. A repo's first
// follow-up is offset by its position so polls spread across the interval.
func (p *Poller) nextRuns(now time.Time, repoKey string, first bool) time.Time {
	interval := p.scale(p.runsInterval(repoKey))
	if !first {
		return now.Add(interval)
	}
	repos := p.st.Repos()
	keys := make([]string, len(repos))
	for i, r := range repos {
		keys[i] = r.Repo.Key()
	}
	slices.Sort(keys)
	i := slices.Index(keys, repoKey)
	return now.Add(interval * time.Duration(i+1) / time.Duration(len(keys)))
}
