# ghtui Slice 4: Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish v1: log search and folding, Runs filtering, a disk cache for instant restarts, `--here`, gentler rate-limit behaviour, plus the dogfooding findings and the most useful deferred minors from slices 1–3.

**Architecture:** No new layers. A small `internal/cache` package persists repos, runs and ETags as JSON; `cmd/ghtui` loads it into the store before the poller starts and saves it after discovery and on quit. The Tail screen gains a line cursor, which folding and search build on. Everything else is a targeted fix in the package that owns the behaviour.

**Tech Stack:** Go 1.25+, existing packages, Bubble Tea v2, Bubbles v2 `textinput`.

**Spec:** `docs/specs/2026-10-07-ghtui-design.md` (§4 rate limit, §6 Runs `/`, Tail `/` `n` `N` `z`, §7 `--here`, §8 cache, §9 offline wording, §11 slice 4)

## Global Constraints

- Tail: `/` searches, `n`/`N` move between hits, `z` folds and unfolds the group under the cursor. Runs: `/` filters by branch or status.
- `ghtui --here`: TUI opens on Runs for the repo inferred from the cwd's `origin` remote.
- Cache at `$XDG_CACHE_HOME/ghtui/` (default `~/.cache/ghtui/`) holds ETags and the last discovery result; cache corruption is treated as a cache miss.
- Network error or 5xx: status bar shows "offline, retrying (data Ns old)"; UI keeps last good state.
- Rate limit: below 500 remaining all intervals double (existing); this slice adds ×4 below 100.
- No change to what needs confirmation: `r`, `x`, `d` still ask `y/N`.
- TDD: failing test, implement, pass, commit. `make build`, `make test`, `make lint` green before each commit. Trunk-based on `main`; push after the final review (the user set up CI and asked to dogfood).

## Review Focus

1. A cache file from an older format, truncated JSON, or an unwritable cache directory must start ghtui normally with a fresh discovery, never crash or block quitting. (Task 5)
2. Search text with regex metacharacters (`[`, `(`, `.`) must match literally and case-insensitively, and searching a 50k-line log must stay under 50 ms per keystroke. (Task 8)
3. Folding a group and then receiving more log lines, or toggling follow, must keep the cursor on a visible line. (Task 7)
4. A Runs filter that matches nothing, followed by a poll update, must keep the cursor valid; `enter`, `r`, `x` do nothing. (Task 9)
5. `--here` outside a git repo or with a non-GitHub remote must exit 2 with a one-line message before the TUI starts. (Task 12)

---

### Task 1: Accurate permission message and strict numbers

**Files:** `internal/gh/errors.go`, `internal/gh/actions_test.go`, `internal/actions/actions.go`, `internal/actions/actions_test.go`, `README.md`

- [ ] Tests: `PermissionError` reads "no permission (403): the token needs write access to this repository — <GitHub message>" and no longer mentions the `workflow` scope (dogfooding showed dispatch, rerun and cancel work without it); `Validate` rejects `NaN`, `Inf`, `0x1p3` and `1e5` for number inputs and accepts `3`, `-2`, `2.5`.
- [ ] Implement; README auth note updated to match.
- [ ] Commit.

### Task 2: CLI fixes

**Files:** `internal/tail/tailer.go`, `internal/tail/tailer_test.go`, `internal/tail/resolve.go`, `internal/tail/resolve_test.go`, `internal/gh/logs.go`, `internal/gh/logs_test.go`, `cmd/ghtui/watch.go`, `cmd/ghtui/watch_test.go`

- [ ] Tests: while a job is in progress the tailer fetches the log only every 6th poll (still works if GitHub ever serves partial logs) and always once the job completes; the blob download is not cut off by the 30 s client timeout (uses its own client with a header timeout); `ResolveJob` on an ID that is neither a job nor a run returns "no job or run 123 in owner/repo"; `watch` prints the run line before the job lines on its first poll and keeps jobs-first ordering afterwards.
- [ ] Implement.
- [ ] Commit.

### Task 3: Poller and store fixes

**Files:** `internal/store/store.go`, `internal/store/store_test.go`, `internal/poller/jobs.go`, `internal/poller/limits.go`, `internal/poller/*_test.go`, `internal/tui/board.go`, `internal/tui/jobs.go`, `internal/tui/statusbar.go`, tests

**Produces:** `RepoState.Polled bool` set by `SetRuns`; poller message `RunsFailed{RepoKey string; Err error}` for transient run-poll failures.

- [ ] Tests: Board shows "loading…" instead of "no runs" for repos not yet polled (dogfooding finding); a focused run whose repo discovery dropped stops the per-tick no-op and Jobs shows "run no longer tracked"; a `RateLimitError` with zero `Reset` pauses 60 s; entries in the backoff map are dropped with their keys; after a transient runs failure the status bar shows "offline, retrying (data 40s old)" using the age of the last successful poll, and clears on the next success.
- [ ] Implement.
- [ ] Commit.

### Task 4: Rate-limit tuning

**Files:** `internal/poller/limits.go`, `internal/poller/limits_test.go`, `internal/tui/statusbar.go`, `internal/tui/statusbar_test.go`

- [ ] Tests: intervals ×2 below 500 remaining and ×4 below 100; the status bar shows "quota low, polling slowed" below 500.
- [ ] Implement.
- [ ] Commit.

### Task 5: Cache package

**Files:** `internal/cache/cache.go`, `internal/cache/cache_test.go`

**Produces:** `type Snapshot struct{ Version int; SavedAt time.Time; Repos []store.RepoState; Runs map[string][]gh.Run }`; `Dir() string` (`$XDG_CACHE_HOME/ghtui`, else `~/.cache/ghtui`); `Load(dir string) (Snapshot, bool)` (false on missing, unreadable, wrong version or corrupt); `Save(dir string, s Snapshot) error` (atomic: temp file + rename, dir created 0700, file 0600); `FromStore(st *store.Store, now time.Time) Snapshot`; `(Snapshot) Apply(st *store.Store)`.

- [ ] Tests: round trip keeps repos, ETags, `Pinned`, runs and order; missing file, truncated JSON, `Version` mismatch and a directory in place of the file all return `false` without error (Review Focus 1); `Save` into an unwritable directory returns an error and leaves no temp file; `Save` replaces atomically (a reader never sees a half file); `Apply` marks restored repos `Polled` so the Board shows their last state, not "loading…".
- [ ] Implement.
- [ ] Commit.

### Task 6: Cache wiring

**Files:** `internal/poller/poller.go`, `internal/poller/poller_test.go`, `cmd/ghtui/tui.go`, `cmd/ghtui/deps.go`, `cmd/ghtui/tui_test.go`

**Produces:** `poller.WithLastDiscovery(t time.Time) Option` so a recent cache delays the first discovery.

- [ ] Tests: with a cache saved under 10 minutes ago the first tick polls runs (sending cached ETags) but not `ListRepos`, and discovery runs at `SavedAt + 10m`; an older cache rediscovers immediately; `runTUI` loads the cache before starting, saves after each discovery and on quit; an unreadable or unwritable cache dir never fails startup or quit (Review Focus 1); `deps.cacheDir` is injectable and tests use a temp dir.
- [ ] Implement.
- [ ] Commit.

### Task 7: Tail cursor and folding

**Files:** `internal/tui/tailview.go`, `internal/tui/tailfold.go`, `internal/tui/tailview_test.go`, `internal/tui/tailfold_test.go`, golden files

- [ ] Tests: a highlighted cursor line; `j`/`k` move the cursor and scroll to keep it visible (follow pauses as today, `G` resumes and puts the cursor on the last line); `z` on a group header or any line inside a group collapses the group to its header with "▸ … (N lines)" and `z` again expands it; `Z` expands all; folded lines are skipped by `j`/`k`; new lines appended to a folded group stay hidden and the count updates; folding near the end with follow on keeps the cursor on a visible line (Review Focus 3); fits 40×10.
- [ ] Implement folding as a set of folded group-start indices applied when building the visible index.
- [ ] Commit.

### Task 8: Tail search

**Files:** `internal/tui/tailsearch.go`, `internal/tui/tailsearch_test.go`, `internal/tui/tailview.go`

- [ ] Tests: `/` opens a search line in the footer (the screen captures keys while it is open); `enter` runs the search and jumps to the first hit at or after the cursor; `esc` closes it; `n`/`N` move to the next and previous hit with wrap-around and show "hit 3 of 12"; hits inside a folded group unfold it; matching is literal and case-insensitive (`[x]`, `a.b`, `(` are not regexes) (Review Focus 2); "no matches for …" when nothing hits; matched text is highlighted in visible lines; a search over 50k lines completes in under 50 ms (Review Focus 2).
- [ ] Implement.
- [ ] Commit.

### Task 9: Runs filter

**Files:** `internal/tui/runs.go`, `internal/tui/runsfilter_test.go`

- [ ] Tests: `/` opens a filter line; typing filters live to runs whose branch or status/conclusion contains the text (case-insensitive); the title shows the filter; `enter` keeps the filter and returns keys to the list; `esc` in the list with a filter clears it; a filter matching nothing shows "no runs match …", and `enter`, `r`, `x`, `w` do nothing (Review Focus 4); a `RunsUpdated` while filtered keeps the selection by ID or clamps.
- [ ] Implement.
- [ ] Commit.

### Task 10: Tail follows a rerun

**Files:** `internal/tui/tailview.go`, `internal/tui/tailview_test.go`

- [ ] Tests: when the open job no longer appears in its run's jobs but a job with the same name does (new attempt), the Tail switches to it: calls `SetTailJob` with the new ID, drops the old log view and shows the new job's steps, and the status bar flashes "following rerun attempt"; if no job with that name exists it keeps showing the old job.
- [ ] Implement.
- [ ] Commit.

### Task 11: Operations polish

**Files:** `internal/tui/dispatchform.go`, `internal/tui/dispatch.go`, `internal/tui/operate.go`, `internal/tui/model.go`, `internal/actions/actions.go`, `cmd/ghtui/tui.go`, tests

- [ ] Tests: the dispatch form ignores `enter` and does not type a second `y` while a dispatch is in flight, and re-enables on error; workflows whose file fails to parse appear greyed in the picker with the parse error and cannot be opened; an open `y/N` prompt is closed when `AuthFailed` arrives; `y` re-checks the run's latest status in the store and refuses ("run is still in progress" / "run already finished") if it changed since the key press; `o` on a run without a URL shows "no page for this run"; the opener uses `rundll32 url.dll,FileProtocolHandler` on Windows.
- [ ] Implement.
- [ ] Commit.

### Task 12: --here, help, README, verification

**Files:** `cmd/ghtui/root.go`, `cmd/ghtui/tui.go`, `cmd/ghtui/tui_test.go`, `internal/tui/model.go`, `internal/tui/help.go`, `README.md`

**Produces:** `tui.NewModelAt(ctx Context, msgs <-chan any, repoKey string) Model` opening on Runs above the Board.

- [ ] Tests: `ghtui --here` in a repo with a GitHub remote opens on Runs for that repo with Board underneath (`esc` returns to Board); outside a git repo or with a non-GitHub remote it exits 2 with one line before any TUI starts (Review Focus 5).
- [ ] Help and README list `/`, `n`, `N`, `z`, `Z`, `--here`, and the cache location.
- [ ] Manual (private tmux socket `-L ghtui-test`): restart ghtui twice and confirm the Board appears instantly from cache; `--here` in this repo; dispatch Dogfood with `outcome=failure`, search the log for "failing", fold a group, rerun and see the Tail follow the new attempt.
- [ ] Commit, then push to `origin/main` after the final review and watch CI with `ghtui watch`.

## Self-review notes

- Spec §11 slice 4 items: search (Task 8), folding (Task 7), disk cache (Tasks 5–6), rate-limit tuning (Task 4), `--here` (Task 12). §6 Runs `/` (Task 9). §9 offline wording (Task 3).
- Dogfooding findings: permission wording (Task 1), "loading…" (Task 3), Tail after rerun (Task 10), `watch` first-poll order (Task 2).
- Deferred minors taken: slice 1 M1, M2, M4 (Task 2); slice 2 focused-run drop, zero reset, failures leak, offline wording (Task 3); slice 3 in-flight form, parse errors, auth prompt, stale status at `y`, empty URL, Windows, strict numbers (Tasks 1, 11).
- Still deferred: main.go print-once test, Esc+reopen refetch race, secondary limits on GETs without Retry-After, YAML aliases, cancellation of in-flight actions on quit, cleared optional inputs sent as "".
- Names: `RepoState.Polled`, `RunsFailed`, `Snapshot`, `WithLastDiscovery`, `NewModelAt`. Consistent.
