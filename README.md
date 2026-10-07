# ghtui

A terminal UI and CLI for GitHub Actions: a live board of runs across your
recently pushed repos, plus `list`, `tail` and `watch` subcommands. Design: [docs/specs/2026-10-07-ghtui-design.md](docs/specs/2026-10-07-ghtui-design.md).

## Install

```sh
make build        # writes bin/ghtui
```

Requires Go 1.25+.

## Auth

ghtui reads a token from `GH_TOKEN`, then `GITHUB_TOKEN`, then
`gh auth token`. If none of those work, run `gh auth login`.

## TUI

```sh
ghtui
```

Opens a board of every repo you pushed to in the last 14 days (plus
`repos.pinned`, minus `repos.exclude`), with each repo's latest run,
running count and failure count. Drill down Board → Runs → Jobs → Log.

| Key | Action |
|---|---|
| `↑`/`k` `↓`/`j` `g` `G` `PgUp` `PgDn` | move or scroll |
| `enter` | open the selected repo, run, job or step |
| `esc` | back |
| `tab` `←` `→` | switch between jobs and steps (Jobs screen) |
| `w` | watch a run: bell and status-bar flash when it finishes |
| `r` | rerun the run: only failed jobs if any failed, else all (asks `y/N`) |
| `x` | cancel the run (asks `y/N`) |
| `d` | dispatch a workflow: pick one with a `workflow_dispatch` trigger, fill the ref and inputs, confirm `y/N` |
| `o` | open the current repo, run or job in the browser |
| `R` | refresh the current screen now |
| `t` | toggle timestamps (log) |
| `G` | jump to the end of the log and follow it |
| `?` | help |
| `q` | quit |

Actions are fire-and-forget: the status bar says "… requested" and the
next poll shows what GitHub did. They need a token with write access to the
repository and the `workflow` scope (`gh auth refresh -s workflow`).

While a job runs, the log screen shows its steps live; GitHub publishes the
log when the job finishes, and it appears then. Polling stays well inside
the API rate limit: runs every 15 s for repos with active runs and 60 s
otherwise, jobs only for the run you are viewing or watching, and ETags so
unchanged lists cost nothing. The status bar shows the remaining quota.

## Commands

Commands that take a repository infer it from the `origin` remote of the
current directory; pass `owner/repo` (or `--repo owner/repo`) to override.

```sh
ghtui list [owner/repo] [--branch B] [--status S] [--limit N] [--json]
```

Recent runs as a table (status, workflow, branch, event, actor, duration,
run number, run ID) or one JSON object per line.

```sh
ghtui tail <run-id|job-id> [--repo owner/repo] [--no-follow] [--timestamps] [--json]
```

Follows a job's log until the job finishes. A run ID resolves to its only
job, or its only in-progress job; otherwise ghtui lists the jobs and asks
for a job ID.

GitHub has no streaming log API, and its job log is only published once
the job completes. In practice `tail` waits, then prints the full log when
the job finishes. It exits 0 on `success`, 1 on any other conclusion, and
2 on usage or API errors.

```sh
ghtui watch <run-id> [--repo owner/repo] [--interval 5s]
```

Prints one line per run or job status change and exits with the run's
conclusion, using the same exit codes as `tail`.

## Config

Optional, at `~/.config/ghtui/config.yaml`:

```yaml
repos:
  pinned: [owner/repo]
  exclude: [owner/archived]
  pushed_within: 14d
poll:
  runs_active: 15s
  runs_idle: 60s
  jobs: 5s   # jobs and watch interval
  logs: 5s   # tail interval
ui:
  show_timestamps: false
```

## Development

```sh
make test
make lint   # builds golangci-lint into bin/ with your Go toolchain
```
