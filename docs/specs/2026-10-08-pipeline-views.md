# Pipeline views: DAG and timeline

One new screen, **Pipeline view**, shows a single run visually. It has two
modes, chosen with `tab`; both are built so they can be compared and one
kept, or both.

## Opening

- `v` on the Runs screen (for the selected run) or on the Pipeline screen
  opens the view. From Runs it also focuses the run so its jobs are polled;
  `esc` restores whatever was focused before.
- `enter` on a job opens its log, `o`/`O`, `c`/`C`, `r`, `x`, `d`, `R` work
  as on the Pipeline screen. `j`/`k` move between jobs; in the DAG, `h`/`l`
  move between columns.

## Data

- Jobs, steps and their times come from the store, as on the Pipeline
  screen. `gh.Job` gains `CreatedAt` (the API's `created_at`) so the wait
  before a job starts is known.
- Dependencies come from the workflow file at the run's commit
  (`head_sha`): `actions.Service.JobGraph` lists the repo's workflows to find
  the file path by workflow ID, fetches the file at the SHA and parses the
  `jobs:` map into `workflow.Job{Key, Name, Needs}` (`needs` may be a scalar
  or a list; `name` is kept verbatim).
- An API job matches a YAML job when its name, minus a matrix suffix
  ` (…)` and a reusable-workflow suffix ` / …`, equals the YAML `name` or
  key. Each matrix instance becomes its own node.
- Jobs that match nothing, or every job when the file could not be read,
  are placed by timing: in the column after the latest job that completed
  before they started. The stats line says `needs from timing` in that case.

## Layout

- **Column** of a job = 1 + the longest chain of `needs` behind it (roots in
  column 0). Jobs in a column keep API order.
- **Critical path**: starting at the last job to finish, repeatedly step to
  the dependency that finished last. Those jobs are shown bold in both
  modes, and the stats line names the chain and its length.

## DAG mode

Jobs are boxes (`┌─┐ │ ✓ name 1m2s │ └─┘`) laid out in columns with a
5-cell gutter. The border takes the status colour; the selected box has a
bold border and reversed content. Edges run from the right edge of the
dependency to the left edge of the dependant: horizontal, a vertical in the
gutter before the target column, horizontal with `▶`. Lines are painted
first and boxes over them, so a long edge passes behind intermediate
boxes. Overlapping lines merge into the right box-drawing junction. The
canvas scrolls to keep the selected box visible.

## Timeline mode

One row per job, ordered by start time (unstarted last). Left: glyph and
name (capped at 24 cells). Right: a bar on a time axis from the run's
`created_at` to its end (or now). `░` from `created_at` to `started_at`
(waiting), then `█` in the status colour while it ran. The axis is a top
line with tick labels every nice interval (10s, 30s, 1m, 5m, …) chosen so
labels do not overlap. The selected job's steps follow it, indented, each
with its own bar; the running step's bar reaches the current time.

## Header

Heading `Graph · owner/repo #n workflow` or `Timeline · …`. Stats line 1 is
the Pipeline screen's; line 2 is `N jobs · M columns · critical path a › b › c
(4m12s)`, with `· needs from timing` when edges were inferred and
`· loading dependencies…` while the file is in flight.
