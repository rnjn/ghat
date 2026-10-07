# ghtui

A terminal UI and CLI for GitHub Actions. Slice 1 ships the CLI: `list`,
`tail` and `watch`. The TUI comes next. Design: [docs/specs/2026-10-07-ghtui-design.md](docs/specs/2026-10-07-ghtui-design.md).

## Install

```sh
make build        # writes bin/ghtui
```

Requires Go 1.25+.

## Auth

ghtui reads a token from `GH_TOKEN`, then `GITHUB_TOKEN`, then
`gh auth token`. If none of those work, run `gh auth login`.

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
poll:
  jobs: 5s   # watch interval
  logs: 5s   # tail interval
ui:
  show_timestamps: false
```

## Development

```sh
make test
make lint   # builds golangci-lint into bin/ with your Go toolchain
```
