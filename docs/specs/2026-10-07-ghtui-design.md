# ghtui: a terminal UI for GitHub Actions

Date: 2026-10-07
Status: approved design, awaiting implementation plan

## 1. Purpose

`ghtui` is a single-binary terminal dashboard for GitHub Actions. It replaces
browser tabs and `gh run watch` for three day-to-day jobs:

- **Watch my pushes.** See a run start, follow its log as steps complete, and
  learn pass/fail without leaving the terminal.
- **Triage failures.** Across all repos the user can access, see what is red,
  jump into the failing job and step log, rerun failed jobs.
- **Operate workflows.** Trigger `workflow_dispatch` with inputs, cancel and
  rerun runs.

It also exposes a small set of pipe-friendly subcommands (`list`, `tail`,
`watch`) that share the same client code, for use from scripts and agents.

### Constraints and assumptions

- GitHub.com only. GitHub Enterprise Server hosts are out of scope for v1.
- Go, Bubble Tea, Lip Gloss. Cobra for the CLI surface.
- Auth is borrowed from the `gh` CLI; no token management of its own.
- Target scale is under 20 active repos across one or two orgs.
- GitHub has no streaming API for job logs. Tailing is poll based and
  advances per completed step, not per line. This is a hard platform limit
  and the design works within it rather than around it (no browser
  WebSocket scraping).
- Notification on completion is in-TUI only: list update, status-line
  flash, terminal bell. No desktop notifications in v1.

### Success criteria

- `ghtui tail <job>` shows new step output within one poll interval of the
  step finishing, and exits with the job's conclusion.
- Opening the TUI shows every active run across the user's recent repos
  within a few seconds, and stays under GitHub's rate limit indefinitely
  while idle.
- Rerun, cancel and dispatch work from the TUI with a one-key confirmation.

## 2. Architecture

Option chosen: **one process, background poller owns all GitHub calls,
in-memory store, UI reads the store.**

```
                 +-----------+   typed msgs   +-------------+
  GitHub API <-> |  poller   | -------------> | Bubble Tea  |
                 | (client)  |                |  program    |
                 +-----+-----+                +------+------+
                       | writes                      | reads
                       v                             v
                 +-----------------------------------------+
                 |                  store                  |
                 +-----------------------------------------+

  CLI subcommands (list/tail/watch) use client + tailer directly,
  no poller, no TUI.
```

Packages:

| Package | Responsibility |
|---|---|
| `internal/gh` | Thin typed REST client. Auth, ETags, rate-limit headers, pagination, 302 log redirect. No polling logic. |
| `internal/store` | Repo → Run → Job → Step tree plus per-job log buffers. Concurrency-safe. Pure data, no I/O. |
| `internal/poller` | Goroutines per resource type with adaptive intervals. Writes to store, emits change messages on a channel. |
| `internal/tail` | Fetch job log, diff against emitted line count, parse `##[group]`, `##[error]`, timestamps. Used by both the poller and `ghtui tail`. |
| `internal/tui` | Bubble Tea models: board, runs, jobs, tail, help, confirm. Reads store only. |
| `internal/config` | YAML config and on-disk cache. |
| `cmd/ghtui` | Cobra root: TUI by default, subcommands otherwise. |

Rejected alternatives: UI-driven fetching (duplicated polling, no CLI
reuse) and a local daemon with socket clients (two processes, overkill for
one user). The store boundary leaves the daemon option open later.

## 3. Data model

```
Repo       { Owner, Name, PushedAt, Pinned, Unavailable, RunsETag }
Run        { ID, RepoKey, Number, WorkflowName, WorkflowID, Branch, Event,
             Actor, Status, Conclusion, CreatedAt, UpdatedAt, HTMLURL,
             Watched, Jobs []JobID }
Job        { ID, RunID, Name, Status, Conclusion, StartedAt, CompletedAt,
             HTMLURL, Steps []Step }
Step       { Number, Name, Status, Conclusion, StartedAt, CompletedAt }
LogBuffer  { JobID, Lines []LogLine, EmittedCount, Complete }
LogLine    { Timestamp, Text, Kind (plain|group|endgroup|error|warning|command),
             StepNumber }
```

`Status` and `Conclusion` use GitHub's strings verbatim (`queued`,
`in_progress`, `completed`; `success`, `failure`, `cancelled`, `skipped`,
`timed_out`, `action_required`, `neutral`, `stale`).

## 4. Polling

| Resource | Endpoint | Interval | Scope |
|---|---|---|---|
| Repo discovery | `GET /user/repos?sort=pushed&per_page=100` | 10 min and on startup | all pages until `pushed_at` older than window |
| Runs per repo | `GET /repos/{o}/{r}/actions/runs?per_page=30` | 15 s if any run active, else 60 s | every discovered repo |
| Jobs for a run | `GET /repos/{o}/{r}/actions/runs/{id}/jobs` | 5 s while in progress, once on completion | selected or watched runs only |
| Job log | `GET /repos/{o}/{r}/actions/jobs/{id}/logs` | 5 s while in progress, once on completion | job open in Tail view, or `ghtui tail` |
| Rate limit | headers on every response | n/a | global |

Rules:

- Discovery result = repos pushed within `pushed_within` (default 14 days)
  ∪ `repos.pinned` − `repos.exclude`.
- Every list call sends `If-None-Match`; a 304 costs no quota and emits no
  message.
- Per-repo run polls are staggered across the interval so requests do not
  burst.
- When `X-RateLimit-Remaining` drops below 500, all intervals double. At 0,
  polling pauses until `X-RateLimit-Reset`; the status bar shows the time.
- Jobs and logs are never polled for runs nobody is looking at.

Messages emitted to the UI: `ReposUpdated`, `RunsUpdated{RepoKey}`,
`JobsUpdated{RunID}`, `LogAppended{JobID, From, To}`, `LogComplete{JobID}`,
`RunCompleted{RunID}` (only for watched runs), `RateLimit{Remaining,
Reset}`, `PollerError{Err, Resource}`.

## 5. Log tail mechanics

The job logs endpoint returns a plain-text file containing all completed
steps so far. For an in-progress job it returns the completed steps; the
currently running step's output appears only when that step finishes. The
run-level zip endpoint 404s until the run completes and is not used.

Algorithm per poll:

1. `GET .../jobs/{id}/logs`, follow the 302 (URL expires in about one
   minute, so never cache it). A 404 on an in-progress job means "no
   completed steps yet" and is not an error.
2. Split into lines. If `len(lines) > EmittedCount`, emit
   `lines[EmittedCount:]` and set `EmittedCount = len(lines)`. Completed
   step output is immutable, so a line-count diff is safe.
3. Parse each new line: strip the leading ISO timestamp into `Timestamp`;
   classify `##[group]`, `##[endgroup]`, `##[error]`, `##[warning]`,
   `##[command]`; attribute to a step by matching group headers against
   step names from the jobs endpoint.
4. If the job is `completed`, do one final fetch after the status flips
   (the log can lag status by a few seconds; retry up to three times) then
   emit `LogComplete`.

Tail view footer always shows the running step's name and elapsed time with
a spinner, so a quiet log never looks frozen.

## 6. TUI

Screens form a stack. `Esc` pops one level. Each screen has a persistent
status bar: active repo, rate-limit remaining, last poll age, last error.

### Board
One row per repo, sorted active-first then by `PushedAt`. Columns: status
glyph of latest run, repo, branch, workflow, age, running count, failed
count (last 30 runs). `Enter` → Runs.

### Runs
Runs for one repo, newest first. Columns: status, workflow, branch, event,
actor, duration, run number. `/` filters by branch or status. `Enter` →
Jobs. Selecting a run starts job polling for it.

### Jobs
Left pane: jobs with status and duration. Right pane: steps of the selected
job with status and duration. `Enter` on a job → Tail from the top. `Enter`
on a step → Tail scrolled to that step's group.

### Tail
Full-screen log for one job. Follow mode on by default; scrolling up pauses
follow, `G` resumes. `/` searches, `n`/`N` move between hits. `z` folds and
unfolds the group under the cursor. `t` toggles timestamps. Errors and
warnings are highlighted.

### Global keys

| Key | Action | Confirm |
|---|---|---|
| `r` | rerun run (failed jobs only if any failed, else all) | yes |
| `x` | cancel run | yes |
| `d` | dispatch workflow: prompt for ref, then each input from the workflow file's `inputs` schema with defaults | yes |
| `w` | toggle watch on run | no |
| `o` | open current row in browser | no |
| `R` | force refresh of current screen | no |
| `?` | help overlay | no |
| `q` | quit | no |

Confirmation is a single-key `y/n` prompt in the status bar. Actions are
fire-and-forget: the UI shows "rerun requested" and the next poll reflects
the real state. A watched run that completes rings the terminal bell and
flashes its conclusion in the status bar for five seconds.

## 7. CLI mode

Given any subcommand, no TUI starts.

- `ghtui` — TUI on Board. `ghtui --here` — TUI on Runs for the repo inferred
  from the cwd's `origin` remote.
- `ghtui list [owner/repo] [--branch B] [--status S] [--limit N] [--json]`
  — runs as a table or JSON lines. Repo defaults to cwd remote.
- `ghtui tail <run-id|job-id> [--repo owner/repo] [--no-follow]` — resolves
  a run ID to its first in-progress job (or the single job, or errors if
  ambiguous and asks for a job ID). Prints new log lines to stdout as steps
  complete until the job finishes. Exit 0 on `success`, 1 on any other
  conclusion, 2 on usage or API error.
- `ghtui watch <run-id> [--repo owner/repo]` — prints one line per status
  or job transition, exits with the run's conclusion using the same codes.

Timestamps are stripped by default; `--timestamps` keeps them. `--json`
on `tail` emits one `LogLine` object per line.

## 8. Config and auth

Auth resolution order: `GH_TOKEN`, `GITHUB_TOKEN`, then `gh auth token`.
Failure exits with one line pointing to `gh auth login`. The token lives in
memory only.

`~/.config/ghtui/config.yaml`, all keys optional:

```yaml
repos:
  pinned: [owner/repo]
  exclude: [owner/archived]
  pushed_within: 14d
poll:
  runs_active: 15s
  runs_idle: 60s
  jobs: 5s
  logs: 5s
ui:
  show_timestamps: false
```

`~/.cache/ghtui/` holds ETags and the last discovery result so restart is
instant and cheap. Cache corruption is treated as cache miss.

## 9. Error handling

| Condition | Behaviour |
|---|---|
| Network error or 5xx | Exponential backoff per resource, cap 2 min. Status bar: "offline, retrying (data 40s old)". UI keeps last good state. |
| Rate limit < 500 | All intervals double. Status bar shows remaining. |
| Rate limit 0 | Polling pauses until reset. Status bar shows reset time. |
| 403/404 on a repo | Repo marked `Unavailable`, skipped until next discovery. |
| 404 on log of in-progress job | "No completed steps yet", not an error. |
| Rerun/cancel/dispatch fails | Error in status bar, no local state change. |
| Auth missing or invalid | Exit before TUI with instruction. 401 mid-session shows a modal and stops polling. |

## 10. Testing

- `internal/gh`: tests against `httptest.Server` with fixtures recorded
  from the real API. Covers pagination, ETag 304, rate-limit headers, the
  302 log redirect, and 404 on in-progress logs.
- `internal/tail`: table tests for line diffing, group and annotation
  parsing, step attribution, and the final-fetch-after-completion retry.
- `internal/poller`: fake clock; assert interval changes on active↔idle
  and on rate-limit drop; assert jobs are not polled for unselected runs.
- `internal/tui`: `teatest` golden tests. Send messages, assert `View()`.
- `cmd/ghtui`: end-to-end `tail` and `watch` exit codes against the fake
  server.
- Tooling: `make build`, `make test`, `make lint` (golangci-lint).
  Test-first per change.

## 11. Delivery slices

1. **Client and CLI.** `internal/gh`, auth, config, `ghtui list`,
   `ghtui tail`, `ghtui watch`. Proves the API model and the tail
   algorithm with no UI.
2. **TUI read path.** Store, poller, Board → Runs → Jobs → Tail, watch
   flag, bell.
3. **Operations.** Rerun, cancel, dispatch with inputs, open in browser,
   confirmations.
4. **Polish.** Search, folding, disk cache, rate-limit backoff tuning,
   `--here`.

Each slice ends with passing tests and a commit on trunk.

## 12. Out of scope for v1

GitHub Enterprise Server, desktop notifications, deployment approvals,
secrets or variables management, artifact download, live per-line log
streaming, multi-account.
