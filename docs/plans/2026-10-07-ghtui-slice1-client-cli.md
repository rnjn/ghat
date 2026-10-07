# ghtui Slice 1: Client and CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A working `ghtui` binary with `list`, `tail` and `watch` subcommands against the real GitHub API, proving the client and the per-step log tail algorithm with no TUI.

**Architecture:** `internal/gh` is a thin typed REST client (auth, ETags, rate-limit headers, pagination, 302 log redirect). `internal/tail` diffs job logs by line count and parses annotations. `cmd/ghtui` wires Cobra subcommands to them. Everything is tested against an `httptest.Server` with recorded fixtures.

**Tech Stack:** Go 1.25, Cobra, `gopkg.in/yaml.v3`, stdlib `net/http`, golangci-lint, Makefile.

**Spec:** `docs/specs/2026-10-07-ghtui-design.md`

## Global Constraints

- Module and binary name `ghtui`; repo directory stays `ghrw`.
- GitHub.com only; base URL `https://api.github.com`, overridable for tests.
- Auth order: `GH_TOKEN`, `GITHUB_TOKEN`, `gh auth token`. Token in memory only.
- Status and conclusion strings are GitHub's verbatim.
- Exit codes: 0 success, 1 non-success conclusion, 2 usage or API error.
- Timestamps stripped by default, `--timestamps` keeps them.
- Every list call sends `If-None-Match` and handles 304.
- TDD: failing test, implement, pass, commit. `make build`, `make test`, `make lint` green before each commit.

## Review Focus

Inputs the spec implies but which are easy to miss. Each is pinned to a test in the owning task.

1. Log fetch 404 on an in-progress job must mean "no steps yet", not failure. (Task 5)
2. Job status flips to `completed` before the final log is available; final fetch must retry. (Task 7)
3. A run with several in-progress jobs given to `tail` must error asking for a job ID, not pick one silently. (Task 7)
4. Rate-limit 403 with `X-RateLimit-Remaining: 0` must surface as a distinct error with reset time, not a generic 403. (Task 3)
5. Lines without a timestamp prefix (rare, but GitHub emits them for some annotations) must parse without panicking. (Task 5)

---

### Task 1: Project scaffold

**Files:** `go.mod`, `Makefile`, `.golangci.yml`, `.gitignore`, `cmd/ghtui/main.go`, `cmd/ghtui/root.go`, `cmd/ghtui/root_test.go`

- [ ] `go mod init ghtui`; add Cobra.
- [ ] Makefile targets: `build` (to `bin/ghtui`), `test` (`go test ./...`), `lint` (`golangci-lint run`), `fmt`.
- [ ] Test: running root with `--version` prints `ghtui dev`.
- [ ] Implement root command with version flag; `main.go` calls `Execute()` and maps errors to exit code 2.
- [ ] `make build test lint` green. Commit.

### Task 2: Auth resolution

**Files:** `internal/gh/auth.go`, `internal/gh/auth_test.go`

**Produces:** `func ResolveToken(env func(string) string, run func(name string, args ...string) ([]byte, error)) (string, error)`

- [ ] Tests: `GH_TOKEN` wins; `GITHUB_TOKEN` second; falls back to `gh auth token` output trimmed; all empty returns `ErrNoAuth` whose message mentions `gh auth login`.
- [ ] Implement with injected env and exec functions.
- [ ] Commit.

### Task 3: HTTP client core

**Files:** `internal/gh/client.go`, `internal/gh/errors.go`, `internal/gh/client_test.go`, `internal/gh/testdata/` (fixtures)

**Produces:** `type Client struct`; `New(token string, opts ...Option) *Client`; `WithBaseURL`, `WithHTTPClient`; `func (c *Client) get(ctx, path string, etag string, out any) (Response, error)` where `Response{ETag string; NotModified bool; RateRemaining int; RateReset time.Time; NextPage string}`; error types `*RateLimitError{Reset}`, `*APIError{Status, Message}`.

- [ ] Tests against `httptest.Server`: sets `Authorization: Bearer`, `Accept`, `X-GitHub-Api-Version` headers; decodes JSON; 304 returns `NotModified` without decoding; parses `X-RateLimit-*` headers; parses `Link: rel="next"`; 403 with remaining 0 returns `*RateLimitError`; other 4xx/5xx return `*APIError` with GitHub's `message`.
- [ ] Implement.
- [ ] Commit.

### Task 4: Actions resources

**Files:** `internal/gh/types.go`, `internal/gh/repos.go`, `internal/gh/runs.go`, `internal/gh/jobs.go`, `internal/gh/*_test.go`, fixtures recorded via `gh api` into `testdata/`

**Produces:** types `Repo`, `Run`, `Job`, `Step` per spec §3; `ListRepos(ctx, pushedSince time.Time) ([]Repo, error)` (paginates until older than cutoff); `ListRuns(ctx, owner, repo string, opts RunsOpts, etag string) ([]Run, Response, error)` with `RunsOpts{Branch, Status string; PerPage int}`; `GetRun(ctx, owner, repo string, id int64) (Run, error)`; `ListJobs(ctx, owner, repo string, runID int64) ([]Job, error)`; `GetJob(ctx, owner, repo string, id int64) (Job, error)`.

- [ ] Record fixtures: one page of repos, runs with branch filter, jobs with steps, a single run, a single job.
- [ ] Tests: each function decodes its fixture; `ListRepos` stops paginating at cutoff; `ListRuns` passes `branch` and `status` query params and forwards ETag.
- [ ] Implement.
- [ ] Commit.

### Task 5: Job log fetch and line parsing

**Files:** `internal/gh/logs.go`, `internal/gh/logs_test.go`, `internal/tail/parse.go`, `internal/tail/parse_test.go`

**Produces:** `func (c *Client) JobLog(ctx, owner, repo string, jobID int64) ([]byte, error)` returning `ErrLogNotReady` on 404; `type LogLine{Timestamp time.Time; Text string; Kind Kind; StepNumber int}`; `Kind` enum `Plain, Group, EndGroup, Error, Warning, Command`; `func ParseLines(raw []byte, steps []gh.Step) []LogLine`.

- [ ] Tests for `JobLog`: follows 302 to a second test server URL without the `Authorization` header; 404 returns `ErrLogNotReady`; body returned verbatim.
- [ ] Tests for `ParseLines`: strips ISO timestamp; classifies each `##[...]` marker; attributes lines to steps by matching `##[group]Run <step name>` headers; line without timestamp parses with zero time (Review Focus 5); empty input returns empty slice; trailing newline does not produce an empty line.
- [ ] Implement.
- [ ] Commit.

### Task 6: Tailer

**Files:** `internal/tail/tailer.go`, `internal/tail/tailer_test.go`

**Produces:** `type Source interface{ JobLog(ctx, owner, repo string, jobID int64) ([]byte, error); GetJob(ctx, owner, repo string, id int64) (gh.Job, error) }`; `type Tailer struct`; `NewTailer(src Source, owner, repo string, jobID int64, opts Options) *Tailer` with `Options{Interval time.Duration; Clock func() time.Time; Sleep func(context.Context, time.Duration) error}`; `func (t *Tailer) Run(ctx, emit func(LogLine)) (gh.Job, error)` returning the final job on completion.

- [ ] Tests with a fake `Source` scripted per call: emits only new lines on growth; emits nothing on equal length; `ErrLogNotReady` is skipped silently (Review Focus 1); stops after job `completed` and returns it; context cancel returns `ctx.Err()`.
- [ ] Implement poll loop: fetch job, fetch log, diff by line count, emit, sleep, repeat.
- [ ] Commit.

### Task 7: Final-fetch retry and run-to-job resolution

**Files:** `internal/tail/tailer.go`, `internal/tail/tailer_test.go`, `internal/tail/resolve.go`, `internal/tail/resolve_test.go`

**Produces:** `func ResolveJob(ctx, c *gh.Client, owner, repo string, id int64) (gh.Job, error)` and `ErrAmbiguousRun`.

- [ ] Test: job reports `completed` but log still short; tailer refetches up to 3 times at interval and emits the late lines (Review Focus 2).
- [ ] Implement retry.
- [ ] Tests for `ResolveJob`: ID matches a job → return it; ID matches a run with one job → that job; run with exactly one `in_progress` job → that job; run with several in-progress jobs → `ErrAmbiguousRun` listing job IDs and names (Review Focus 3); neither → `*APIError` 404 passthrough.
- [ ] Implement: try `GetJob`, on 404 try `ListJobs` for run.
- [ ] Commit.

### Task 8: Config and repo inference

**Files:** `internal/config/config.go`, `internal/config/config_test.go`, `internal/config/repo.go`, `internal/config/repo_test.go`

**Produces:** `type Config` per spec §8 with defaults; `Load(path string) (Config, error)` (missing file → defaults); `func InferRepo(remoteURL string) (owner, repo string, err error)`; `func CurrentRepo(run func(name string, args ...string) ([]byte, error)) (owner, repo string, err error)` using `git remote get-url origin`.

- [ ] Tests: defaults when file missing; YAML overrides; `14d` duration parses; bad YAML returns error.
- [ ] Tests for `InferRepo`: `git@github.com:o/r.git`, `https://github.com/o/r`, `https://github.com/o/r.git`, non-GitHub host → error.
- [ ] Implement.
- [ ] Commit.

### Task 9: `ghtui list`

**Files:** `cmd/ghtui/list.go`, `cmd/ghtui/list_test.go`, `cmd/ghtui/output.go`, `cmd/ghtui/output_test.go`, `cmd/ghtui/deps.go`

- [ ] `deps.go`: construct `*gh.Client` from `ResolveToken`; allow test override of base URL via hidden `--api-url` flag.
- [ ] Tests: table output has header and one row per run with status, workflow, branch, event, actor, duration, number; `--json` emits one JSON object per line; repo arg optional and inferred from cwd; `--branch`, `--status`, `--limit` forwarded; API error exits 2.
- [ ] Implement.
- [ ] Commit.

### Task 10: `ghtui tail`

**Files:** `cmd/ghtui/tail.go`, `cmd/ghtui/tail_test.go`

- [ ] Tests against fake server scripted to grow the log over three polls: prints new lines only; strips timestamps by default; `--timestamps` keeps them; `--json` emits `LogLine` JSON; `--no-follow` prints once and exits; exit 0 on `success`, 1 on `failure`, 2 on `ErrAmbiguousRun` with the job list on stderr; `--repo` overrides inference.
- [ ] Implement using `ResolveJob` and `Tailer`.
- [ ] Commit.

### Task 11: `ghtui watch`

**Files:** `cmd/ghtui/watch.go`, `cmd/ghtui/watch_test.go`

- [ ] Tests: prints one line per run status change and per job status change, format `<time> run <status>` / `<time> job <name> <status>[ <conclusion>]`; silent when nothing changed; exits with conclusion code when run completes; `--interval` flag honoured.
- [ ] Implement: poll `GetRun` and `ListJobs`, diff against previous snapshot.
- [ ] Commit.

### Task 12: Manual verification and README

**Files:** `README.md`

- [ ] Run `ghtui list` against a real repo of yours; confirm output.
- [ ] Trigger a real run, `ghtui tail <run-id>`; confirm lines appear as steps finish and exit code matches.
- [ ] `ghtui watch <run-id>` on the same run.
- [ ] README: install, auth, the three commands, link to spec. Commit.

## Self-review notes

- Spec §7 covered by Tasks 9–11; §5 by 5–7; §8 by 2 and 8; §9 rows relevant to CLI (rate limit, 404 log, auth) by 3, 5, 2. §4 polling, §6 TUI, and the rest of §9 are slice 2.
- `Response`, `LogLine`, `Source`, `ResolveJob` names used consistently across tasks.
