# ghtui Slice 2: TUI Read Path Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Running `ghtui` with no subcommand opens a TUI that shows live runs across the user's recent repos, drills into runs, jobs and steps, shows live step progress for running jobs and the full log once a job completes, and rings the bell when a watched run finishes.

**Architecture:** A background poller owns every GitHub call and writes into a concurrency-safe in-memory store, then sends typed change messages to the Bubble Tea program. The poller is a deterministic scheduler with a `Tick(now)` method (driven by a real ticker in production, called directly in tests). TUI screens read only the store and render on messages.

**Tech Stack:** Go 1.25+, Bubble Tea v2 (`charm.land/bubbletea/v2`), Lip Gloss v2 (`charm.land/lipgloss/v2`), Bubbles v2 (`charm.land/bubbles/v2`), teatest v2 (`github.com/charmbracelet/x/exp/teatest/v2`), existing `internal/gh`, `internal/tail`, `internal/config`.

**Spec:** `docs/specs/2026-10-07-ghtui-design.md` (§2, §3, §4, §5, §6 read-only parts, §9; updated 2026-10-07 with the log-at-completion finding)

## Global Constraints

- In scope: store, poller, Board → Runs → Jobs → Tail, help overlay, status bar, watch flag (`w`), bell and 5 s flash on watched-run completion, force refresh (`R`), quit (`q`), `Esc` pops a screen.
- Out of scope (later slices): rerun/cancel/dispatch/open (`r` `x` `d` `o`), confirmations, search (`/` `n` `N`), folding (`z`), disk cache, `--here`.
- Polling per spec §4: discovery 10 min and on start; runs 15 s if any run active, else 60 s; jobs 5 s for focused or watched runs only; logs only for the job open in Tail, once completed.
- Discovery = repos pushed within `pushed_within` ∪ `repos.pinned` − `repos.exclude`.
- Every runs list call sends `If-None-Match`; a 304 emits no message.
- Per-repo run polls staggered across the interval.
- Rate limit: remaining < 500 doubles all intervals; 0 pauses until reset; status bar shows remaining and reset.
- Errors per spec §9: network/5xx exponential backoff per resource capped at 2 min, UI keeps last good state; 403/404 on a repo marks it `Unavailable` until next discovery; 401 shows a modal and stops polling.
- Status and conclusion strings are GitHub's verbatim.
- TDD: failing test, implement, pass, commit. `make build`, `make test`, `make lint` green before each commit. Trunk-based, commit on `main`.

## Review Focus

1. A repo with Actions disabled or no workflows (runs list 404, or zero runs) must show on the Board as unavailable or empty, never crash or block other repos. (Task 6)
2. A tiny terminal (40×10) or a resize mid-session must truncate cleanly, never panic on negative widths. (Task 7, Task 10)
3. The selected run or job vanishing between polls (new rerun attempt, list shrinks) must clamp the cursor, not index out of range. (Task 8, Task 9)
4. Quitting while requests are in flight must exit promptly with no send on a closed channel. (Task 12)
5. A 50k-line completed log must scroll smoothly: render only the visible window. (Task 10)

---

### Task 1: Spec update, client rate-limit accessor, GetRepo

**Files:** `docs/specs/2026-10-07-ghtui-design.md`, `internal/gh/client.go`, `internal/gh/client_test.go`, `internal/gh/repos.go`, `internal/gh/resources_test.go`

**Produces:** `func (c *Client) RateLimit() (remaining int, reset time.Time)` updated from every response (remaining −1 until first response); `func (c *Client) GetRepo(ctx, owner, repo string) (Repo, error)` for pinned repos outside the push window.

- [ ] Commit the spec update (log only available at job completion; Tail shows step progress while running).
- [ ] Tests: `RateLimit()` reflects headers of the latest response, including error responses; safe under concurrent calls (`go test -race`); `GetRepo` decodes `owner`, `name`, `pushed_at`, `archived`.
- [ ] Implement with a mutex-guarded field on `Client`.
- [ ] Add `-race` to `make test`. Commit.

### Task 2: Store: repos, runs, jobs

**Files:** `internal/store/store.go`, `internal/store/store_test.go`

**Consumes:** `gh.Repo`, `gh.Run`, `gh.Job`.

**Produces:** `type Store struct`; `New() *Store`; `type RepoState{Repo gh.Repo; Pinned, Unavailable bool; RunsETag string; LastError string}`; `SetRepos([]RepoState)` (keeps existing ETags for repos still present, clears `Unavailable`); `Repos() []RepoState` sorted active-first then `PushedAt` desc; `SetRuns(repoKey string, runs []gh.Run, etag string) (completedWatched []gh.Run)`; `Runs(repoKey string) []gh.Run` newest first; `Run(id int64) (gh.Run, bool)`; `SetJobs(runID int64, jobs []gh.Job)`; `Jobs(runID int64) []gh.Job`; `MarkUnavailable(repoKey, reason string)`; `ToggleWatch(runID int64) bool`; `Watched(runID int64) bool`; `WatchedRuns() []int64`; `AnyActive(repoKey string) bool`.

- [ ] Tests: getters return copies (mutating a result does not change the store); `SetRuns` returns a watched run exactly once when it moves to `completed`, never for unwatched runs; watched runs that complete are unwatched automatically; `SetRepos` preserves ETags and drops removed repos and their runs; sort order of `Repos()`; `AnyActive` true for `queued`/`in_progress`/`waiting`/`requested`/`pending`; concurrent readers and writers under `-race`.
- [ ] Implement with one `sync.RWMutex`.
- [ ] Commit.

### Task 3: Store: focus, tail target, log buffers

**Files:** `internal/store/focus.go`, `internal/store/logs.go`, `internal/store/focus_test.go`, `internal/store/logs_test.go`

**Consumes:** `tail.LogLine`.

**Produces:** `SetFocusRun(runID int64)` (0 clears); `FocusRun() int64`; `SetTailJob(owner, repo string, jobID int64)` (0 clears); `TailJob() (owner, repo string, jobID int64)`; `type LogBuffer{JobID int64; Lines []tail.LogLine; Complete bool}`; `SetLog(jobID int64, lines []tail.LogLine, complete bool) (from, to int)` which keeps the existing prefix and returns the appended range; `Log(jobID int64) LogBuffer`; `DropLog(jobID int64)`.

- [ ] Tests: `SetLog` with a longer log returns the new range and keeps old lines identical; with an equal or shorter log returns an empty range and changes nothing; `Complete` sticks once true; clearing the tail job drops its buffer; logs for at most the 5 most recent tail jobs are kept (older ones dropped) so memory stays bounded.
- [ ] Implement.
- [ ] Commit.

### Task 4: Poller core: messages, scheduler, discovery, runs

**Files:** `internal/poller/msgs.go`, `internal/poller/poller.go`, `internal/poller/schedule.go`, `internal/poller/poller_test.go`, `internal/poller/fake_test.go`

**Consumes:** Store (Tasks 2–3), `config.Config`, `gh` client methods.

**Produces:** `type API interface{ ListRepos; GetRepo; ListRuns; ListJobs; GetJob; JobLog; RateLimit }` (signatures as in `internal/gh`); messages `ReposUpdated{}`, `RunsUpdated{RepoKey string}`, `JobsUpdated{RunID int64}`, `LogAppended{JobID int64; From, To int}`, `LogComplete{JobID int64}`, `RunCompleted{Run gh.Run}`, `RateLimit{Remaining int; Reset time.Time}`, `PollerError{Resource string; Err error}`, `AuthFailed{Err error}`; `New(api API, st *store.Store, cfg config.Config, send func(any)) *Poller`; `(*Poller) Tick(ctx, now time.Time)` runs every due job synchronously; `(*Poller) Run(ctx)` ticks once a second until ctx is done; `(*Poller) Refresh(resource string)` marks resources due now (`"repos"`, `"runs:<owner/repo>"`, `"jobs:<runID>"`, `"log:<jobID>"`).

- [ ] Fake API in `fake_test.go`: scripted responses per method, records calls with arguments and order.
- [ ] Tests (fake clock via explicit `now`): first `Tick` runs discovery then polls runs for every discovered repo; discovery result applies pinned (via `GetRepo` when not discovered), exclude and the `pushed_within` cutoff; discovery repeats after 10 min, not before; runs interval is 15 s when the repo has an active run and 60 s otherwise; with 4 repos and a 60 s interval the first polls are staggered 15 s apart; the stored ETag is sent and a 304 sends no message; changed runs send `RunsUpdated{RepoKey}`; a watched run completing sends `RunCompleted` once.
- [ ] Implement the scheduler as a map of resource key → next-due time; each Tick runs keys with due ≤ now in a stable order.
- [ ] Commit.

### Task 5: Poller: jobs and logs on demand

**Files:** `internal/poller/jobs.go`, `internal/poller/logs.go`, `internal/poller/jobs_test.go`, `internal/poller/logs_test.go`

**Consumes:** `store.FocusRun`, `store.WatchedRuns`, `store.TailJob`, `store.SetLog`, `tail.ParseLines`.

- [ ] Tests: no `ListJobs` call for runs that are neither focused nor watched; focused in-progress run polls jobs every 5 s and sends `JobsUpdated`; a completed run's jobs are fetched once more after completion, then stop; the tail job's log is not fetched while the job is in progress (the blob does not exist yet); once completed it is fetched, parsed, stored, and `LogAppended` plus `LogComplete` are sent; `ErrLogNotReady` after completion retries up to 3 times at the jobs interval, then sends `PollerError{Resource: "log:<id>"}`; changing the tail job stops polling the old one.
- [ ] Implement. The tail job's status comes from the focused run's jobs in the store; if the job is not in the store yet, call `GetJob`.
- [ ] Commit.

### Task 6: Poller: rate limit, backoff, unavailable repos, auth

**Files:** `internal/poller/limits.go`, `internal/poller/limits_test.go`, `internal/poller/poller.go`

- [ ] Tests: when `RateLimit()` remaining < 500 every interval doubles and `RateLimit` msg is sent; at 0 nothing is polled until the reset time passes; a 5xx or network error on one resource backs off 2×, 4×, … capped at 2 min, and other resources keep polling; success resets the backoff; a 404 or 403 on a repo's runs marks it `Unavailable` and skips it until the next discovery (Review Focus 1); a repo with zero runs is stored with an empty list and no error (Review Focus 1); a 401 from any call sends `AuthFailed` once and stops all polling; `*gh.RateLimitError` pauses until its reset.
- [ ] Implement.
- [ ] Commit.

### Task 7: TUI shell: root model, screen stack, status bar, help, Board

**Files:** `internal/tui/model.go`, `internal/tui/keys.go`, `internal/tui/styles.go`, `internal/tui/statusbar.go`, `internal/tui/help.go`, `internal/tui/board.go`, `internal/tui/model_test.go`, `internal/tui/board_test.go`, `internal/tui/testdata/*.golden`

**Consumes:** Store, poller messages, `poller.Refresh`.

**Produces:** `type Screen interface{ Update(tea.Msg, *Context) (Screen, tea.Cmd); View(width, height int) string; Title() string }`; `type Context struct{ Store *store.Store; Refresh func(string); Now func() time.Time }`; `NewModel(ctx Context, msgs <-chan any) Model`; `type Push struct{ Screen Screen }` message; `Esc` pops; `q` quits; `?` toggles the help overlay listing only keys implemented in this slice.

- [ ] Add Bubble Tea v2, Lip Gloss v2, Bubbles v2, teatest v2 at latest.
- [ ] Status bar tests: shows screen title, rate-limit remaining, last poll age, last error text; shows "rate limited until HH:MM" when paused; truncates to width.
- [ ] Board tests (golden, fixed clock): one row per repo with latest-run glyph, repo, branch, workflow, age, running count, failed count over the stored runs; active repos first; unavailable repos dimmed with reason; `j`/`k`/arrows move the cursor; `Enter` pushes Runs for the selected repo; empty store shows "discovering repos…".
- [ ] Test: `View` at 40×10 and at 0×0 does not panic and no line exceeds the width (Review Focus 2).
- [ ] Implement. Poller messages arrive through a `tea.Cmd` that reads one message from the channel and re-arms itself.
- [ ] Commit.

### Task 8: Runs screen

**Files:** `internal/tui/runs.go`, `internal/tui/runs_test.go`, golden files

- [ ] Tests: columns status, workflow, branch, event, actor, duration, run number (newest first); `RunsUpdated` for this repo re-renders and keeps the cursor on the same run ID; when that run disappears the cursor clamps to the list (Review Focus 3); `Enter` sets the focus run in the store, calls `Refresh("jobs:<id>")` and pushes Jobs; `Esc` clears the focus run; `w` toggles watch and shows a watch marker on the row.
- [ ] Implement.
- [ ] Commit.

### Task 9: Jobs screen

**Files:** `internal/tui/jobs.go`, `internal/tui/jobs_test.go`, golden files

- [ ] Tests: left pane lists jobs with status and duration, right pane lists the selected job's steps with status and duration; `Tab` or `←`/`→` switches pane focus; running step shows elapsed time from the injected clock; `JobsUpdated` keeps the selected job by ID, clamping when it vanishes (Review Focus 3); `Enter` on a job pushes Tail at the top; `Enter` on a step pushes Tail targeting that step number; pushing Tail calls `store.SetTailJob`.
- [ ] Implement with a horizontal split that collapses to the jobs pane alone below 60 columns.
- [ ] Commit.

### Task 10: Tail screen

**Files:** `internal/tui/tailview.go`, `internal/tui/tailview_test.go`, golden files

- [ ] Tests, job running: body shows the step list with status glyphs and the running step's elapsed time; a footer spinner shows the running step name; a line reads "log is published when the job finishes".
- [ ] Tests, job completed: on `LogAppended` the body shows log lines; follow mode keeps the last line visible as lines arrive; `k`/`PgUp` pause follow, `G` resumes; `t` toggles timestamps; `##[error]` lines styled as errors and `##[warning]` as warnings; groups render with a `▸` header; opening with a target step scrolls to that step's first line; `PollerError` for this log shows "log not available yet / expired" in the body.
- [ ] Test: a 50k-line buffer renders in under 50 ms and `View` only formats the visible window (Review Focus 5).
- [ ] Test: resize to 40×10 mid-view keeps scroll position valid (Review Focus 2).
- [ ] Implement with a hand-rolled window over `store.Log(jobID).Lines` (not a viewport holding the full string).
- [ ] Commit.

### Task 11: Watch, bell, flash, force refresh, auth modal

**Files:** `internal/tui/model.go`, `internal/tui/statusbar.go`, `internal/tui/notify_test.go`

- [ ] Tests: `RunCompleted` emits a bell (`\a` via `tea.Printf` or the v2 equivalent) and the status bar flashes "<repo> #<n> <workflow>: <conclusion>" for 5 s, then clears (injected clock plus a tick message); `R` calls `Refresh` for the current screen's resource; `AuthFailed` shows a full-screen modal "GitHub rejected the token. Run `gh auth login`, then restart ghtui." and only `q` works.
- [ ] Implement.
- [ ] Commit.

### Task 12: Wire `ghtui` to the TUI, shutdown, verification

**Files:** `cmd/ghtui/root.go`, `cmd/ghtui/tui.go`, `cmd/ghtui/tui_test.go`, `README.md`

- [ ] Root command with no subcommand: resolve token (exit 2 with the `gh auth login` line on failure, before any TUI), load config, build client, store, poller, model; run poller and program under one context.
- [ ] Message channel is buffered; the poller's `send` selects on ctx so it never blocks or sends after shutdown.
- [ ] Test: start with a fake API, send `q`, program and poller both return within 1 s, no panic, no goroutine left (Review Focus 4; check with `runtime.NumGoroutine` settling or `goleak` if the dependency is acceptable).
- [ ] Manual: run `ghtui` against real repos; confirm Board fills within a few seconds, drill to a running job, see step progress, wait for completion, see the log; watch a run and hear the bell; leave idle 10 minutes and confirm rate-limit remaining stays roughly flat.
- [ ] README: TUI section with keys. Commit.

## Self-review notes

- Spec §2 packages store, poller, tui: Tasks 2–12. §3 data model: Tasks 2–3 (`RepoState` carries `Pinned`, `Unavailable`, `RunsETag`). §4 polling and messages: Tasks 4–6. §5 log algorithm for the TUI: Task 5. §6 read-only screens and keys `w` `R` `?` `q` `Esc`: Tasks 7–11. §9 error rows: Task 6 and Task 11. §10 testing: fake clock (Tick), teatest goldens.
- Deferred to slice 3/4 by spec §11: `r` `x` `d` `o`, confirmations, `/` `n` `N` `z`, disk cache, `--here`.
- Names used across tasks: `Store`, `RepoState`, `LogBuffer`, `SetLog`, `TailJob`, `FocusRun`, `API`, `Tick`, `Refresh`, `Screen`, `Context`, `Push`, message types. Consistent.
