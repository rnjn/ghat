<p align="center">
  <img src="docs/assets/icon.svg" width="96" alt="ghat: a status board icon">
</p>

<h1 align="center">ghat</h1>

<p align="center">
  A terminal dashboard and CLI for GitHub Actions.<br>
  Watch your pushes, triage failures and operate workflows without leaving the terminal.
</p>

<p align="center">
  <a href="https://github.com/rnjn/ghat/actions/workflows/ci.yml"><img src="https://github.com/rnjn/ghat/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
</p>

```text
 ghat ▸ Runs · rnjn/ghat                                                     quota 4932 · 09:13
 12 runs · 0 active · 83% success of 12 finished · median 55s
 last failure 11h ago · 0 watched · default branch main
────────────────────────────────────────────────────────────────────────────────────────────────
  STATUS     W  RUN  COMMIT   DURATION  WORKFLOW                                  BRANCH  EVENT…
› ✓ success     #2   e2c2316  1m38s     Push on main                              main    dynam…
  ✓ success     #7   e2c2316  52s       CI                                        main    push …
  ✓ success     #1   e62e3a3  1m40s     CodeQL Setup                              main    dynam…
  ✓ success     #1   e62e3a3  38s       Graph Update: go_modules in /. #1616375…  main    dynam…
  ✓ success     #6   e62e3a3  55s       CI                                        main    push …
  ✓ success     #5   5400fa5  1m9s      CI                                        main    push …
  ✓ success     #4   2838c79  44s       CI                                        main    push …
  ✓ success     #3   df1b487  43s       CI                                        main    push …
  ✓ success     #2   b560f24  49s       CI                                        main    push …
  ✗ failure     #2   f9b86a3  1m37s     Dogfood                                   main    workf…
 polled 0s ago
```

```text
 ghat ▸ Pipeline · rnjn/ghat #2 CI                                                      22:26
 ✓ success · main · push · rnjn · 49s
 jobs 3/3 done · 0 failed
────────────────────────────────────────────────────────────────────────────────────────────────
     JOB    DURATION                   │      STEP                          DURATION
› ✓  test   44s                        │   ✓  Set up job                    1s
  ✓  build  21s                        │   ✓  Run actions/checkout@v7       1s
  ✓  lint   19s                        │   ✓  Run actions/setup-go@v7       11s
                                       │   ✓  Test                          26s
                                       │   ✓  Post Run actions/setup-go@v7  0s
                                       │   ✓  Post Run actions/checkout@v7  0s
                                       │   ✓  Complete job                  0s
 polled 0s ago
```

`v` on a run draws it as a graph or, with `tab`, as a timeline:

```text
 ghat ▸ Graph · rnjn/ghat #4 Dogfood                                                      13:37
 ✓ success · main · workflow_dispatch · rnjn · 1m44s
 3 jobs · 2 columns · critical path slow › report (1m43s)
────────────────────────────────────────────────────────────────────────────────────────────────

 ┌──────────────┐     ┌─────────────┐
 │ ✓ slow 1m34s │───┬▶│ ✓ report 3s │
 └──────────────┘   │ └─────────────┘
                    │
 ┌──────────────┐   │
 │ ✓ fast    3s │───╯
 └──────────────┘

 tab: timeline · ←/→ columns · enter: log
 polled 0s ago
```

```text
 ghat ▸ Timeline · rnjn/ghat #4 Dogfood                                                   13:37
 ✓ success · main · workflow_dispatch · rnjn · 1m44s
 3 jobs · 2 columns · critical path slow › report (1m43s)
────────────────────────────────────────────────────────────────────────────────────────────────
                   0                  30s                1m                 1m30s
                   ┬──────────────────┬──────────────────┬──────────────────┬─────────
  ✓ slow           ░░█████████████████████████████████████████████████████████████     1m34s
  ✓ fast           ░░██                                                                3s
› ✓ report                                                                        ░██  3s
    ✓ Set up job                                                                    █  0s
    ✓ Summarise                                                                     █  0s
    ✓ Complete job                                                                  █  0s

 tab: graph · enter: log
 polled 0s ago
```

`░` is the wait before a job started, `█` its run time; the selected job's steps appear beneath it.

## Features

- **One board for every repo you work on.** Repos you pushed to in the last 14 days, plus pinned ones, with each repo's latest run, running and failed counts.
- **Drill down.** Repositories → Runs → Pipeline (jobs and steps) → Logs, with a header of live stats on every screen.
- **Jump to the change.** Each run shows its commit; the short SHA is a clickable link in terminals that support hyperlinks, `c` copies the commit's diff link and `C` opens it.
- **See the pipeline.** `v` draws a run as a dependency graph (jobs as boxes, `needs:` as arrows, read from the workflow file at the run's commit) or, with `tab`, as a timeline: queue wait, run time and the selected job's steps as bars on one axis. Both mark the critical path — the chain of jobs that set the run's length.
- **Live step progress and logs.** Running jobs show each step's status and elapsed time; the full log appears the moment the job finishes, with errors and warnings highlighted.
- **Search and fold logs.** `/` to search, `n`/`N` to jump between hits, `z` to fold a step's group.
- **Operate.** Rerun (failed jobs only when any failed), cancel, and dispatch workflows with their inputs, each behind a `y/N` confirmation.
- **Watch a run.** `w` rings the bell and flashes the result when it finishes.
- **Scriptable CLI.** `ghat list`, `ghat tail` and `ghat watch` with exit codes that follow the run's conclusion.
- **Gentle on the API.** ETags, staggered polling, polling only what you look at, automatic slow-down when the rate limit runs low, and a disk cache for instant restarts.

## Install

```sh
go install github.com/rnjn/ghat/cmd/ghat@latest
```

Or from a checkout: `make build` writes `bin/ghat`. Requires Go 1.25+.

`ghat --version` and the bottom-right corner of the TUI show what you are
running: the release (e.g. `v0.1.1`), plus the commit for builds from
`@main` or a checkout (a `*` marks uncommitted changes).

## Auth

ghat uses your GitHub CLI login. It reads a token from `GH_TOKEN`, then
`GITHUB_TOKEN`, then `gh auth token`; if none works, run `gh auth login`.
The token stays in memory. Rerun, cancel and dispatch need write access to
the repository (the `repo` scope `gh auth login` grants is enough).

## The TUI

```sh
ghat          # Repositories: every repo you pushed to recently
ghat --here   # straight to the runs of the repo in the current directory
```

| Screen | Shows | Header stats |
|---|---|---|
| Repositories | each repo's latest run, running and failed counts | repos, unavailable, active runs, success rate, failures, watched |
| Runs | a repo's runs: status, run number, commit, duration, workflow, branch, event, actor | runs, active, success rate, median duration, last failure |
| Pipeline | a run's jobs, and the selected job's steps | status, branch, event, actor, duration, jobs done, current step |
| Graph / Timeline | a run as a dependency graph, or as bars on a time axis | jobs, columns, critical path and its length, whether edges came from `needs:` or from timing |
| Logs | live steps while a job runs, then its log | job status, steps done, running step, lines, search hits |

| Key | Action |
|---|---|
| `↑`/`k` `↓`/`j` `g` `G` `PgUp` `PgDn` | move or scroll |
| `enter` | open the selected repo, run, job or step |
| `esc` | back (or close a search or filter) |
| `tab` `←` `→` | switch between jobs and steps; on the graph, `tab` switches graph / timeline and `←` `→` move between columns |
| `v` | view the run as a graph or timeline (from Runs or Pipeline) |
| `/` | search the log (literal, case-insensitive); on Runs, filter by branch or status |
| `n` `N` | next / previous search hit |
| `z` `Z` | fold or unfold the group under the cursor / unfold all |
| `t` | toggle timestamps |
| `w` | watch a run: bell and flash when it finishes |
| `r` | rerun: only failed jobs if any failed, else all (asks `y/N`) |
| `x` | cancel the run (asks `y/N`) |
| `d` | dispatch a workflow: pick one, fill the ref and inputs, confirm `y/N` |
| `o` / `O` | copy the link of the current repo, run or job / open it in the browser |
| `c` / `C` | copy the link of the run's commit (its diff) / open it in the browser |
| `R` | refresh the current screen now |
| `?` | help |
| `q` | quit |

Copying uses the terminal's clipboard escape (OSC 52), so the link lands
on the machine you are sitting at, even over SSH. Most modern terminals
support it; inside tmux add `set -g set-clipboard on`. The status bar also
shows the copied link, so you can select it by hand where copying is
unsupported.

Actions are fire-and-forget: the status bar says "… requested" and the next
poll shows what GitHub did.

## The CLI

Each command takes `owner/repo` (or `--repo owner/repo`); without it,
the repo comes from the current directory's `origin` remote.

```sh
ghat list [owner/repo] [--branch B] [--status S] [--limit N] [--json]
```

Recent runs as a table (status, workflow, branch, event, actor, duration,
run number, run ID, commit) or one JSON object per line.

```sh
ghat tail <run-id|job-id> [--repo owner/repo] [--no-follow] [--timestamps] [--json]
```

Waits for a job and prints its log. A run ID resolves to its only job or its
only running job; otherwise ghat lists the jobs and asks for a job ID.

```sh
ghat watch <run-id> [--repo owner/repo] [--interval 5s]
```

Prints one line per run or job status change until the run finishes.

`tail` and `watch` exit 0 on `success`, 1 on any other conclusion, and 2
on usage or API errors, so they work in scripts: `ghat watch $id && deploy`.

### About live logs

GitHub has no streaming log API, and it publishes a job's log only once the
whole job has finished. While a job runs, ghat shows each step's status and
elapsed time; the log appears as soon as GitHub publishes it.

## Configuration

Optional, at `~/.config/ghat/config.yaml`:

```yaml
repos:
  pinned: [owner/repo]       # always on the board
  exclude: [owner/archived]  # never on the board
  pushed_within: 14d         # how recent a push puts a repo on the board
poll:
  runs_active: 15s           # repos with a running or queued run
  runs_idle: 60s
  jobs: 5s                   # the run you are viewing or watching
  logs: 5s                   # ghat tail
  runs_per_repo: 50          # runs shown per repo in the TUI (1–100)
ui:
  show_timestamps: false
```

ghat was called ghtui before 2026-10-08; a config left at
`~/.config/ghtui/config.yaml` is still read until you move it.

Repos, runs and ETags are cached in `~/.cache/ghat/` (or
`$XDG_CACHE_HOME/ghat/`), so a restart shows the board at once; a broken
cache is ignored. Polling slows ×2 below 500 remaining API requests and ×4
below 100, and the header shows the remaining quota.

## Development

```sh
make test    # go test -race ./...
make lint    # golangci-lint, built into bin/ with your Go toolchain
make build
```

CI runs test, lint and build on every push. The manual
[Dogfood](.github/workflows/dogfood.yml) workflow has timed steps, annotations
and a selectable outcome, for trying ghat against real runs.

The design lives in [docs/specs](docs/specs/2026-10-07-ghtui-design.md) and
the implementation plans in [docs/plans](docs/plans).
