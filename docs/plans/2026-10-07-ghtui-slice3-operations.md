# ghtui Slice 3: Operations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** From the TUI, rerun (`r`), cancel (`x`), dispatch with inputs (`d`) and open in browser (`o`), with a one-key `y/n` confirmation for the three that change anything on GitHub.

**Architecture:** `internal/gh` gains write calls and workflow-file access. `internal/workflow` parses a workflow file's `workflow_dispatch` inputs. `internal/actions` validates and performs each operation and turns results into messages. TUI screens issue actions as `tea.Cmd`s through a small `Actions` interface on `tui.Context`, so the UI never blocks and tests use a fake. Results are fire-and-forget: the status bar says "rerun requested", and a forced refresh of the repo's runs lets the next poll show the real state.

**Tech Stack:** Go 1.25+, existing `internal/gh`, `gopkg.in/yaml.v3`, Bubble Tea v2, Bubbles v2 `textinput`.

**Spec:** `docs/specs/2026-10-07-ghtui-design.md` (§1 "Operate workflows", §6 global keys and confirmation, §9 "Rerun/cancel/dispatch fails")

## Global Constraints

- Keys: `r` rerun (failed jobs only if any job failed, else all jobs), `x` cancel, `d` dispatch, `o` open in browser. `r`, `x`, `d` require `y` to confirm; any other key cancels the prompt. `o` needs no confirmation.
- Confirmation is a single-key `y/n` prompt in the status bar.
- Fire-and-forget: on success show "<action> requested" in the status bar and refresh that repo's runs (and the focused run's jobs); never edit the store locally.
- On failure show the error in the status bar; no local state change.
- Dispatch prompts for the ref (default: the selected run's branch, else the repo's default branch), then each input from the workflow file's `on.workflow_dispatch.inputs`, pre-filled with defaults.
- Writes go through the user's token; a 403 must say the token lacks permission (needs repo write and the `workflow` scope), not show a bare "403".
- Architecture note: spec §2 says the poller owns all GitHub calls. Reads stay in the poller; user-triggered writes (and the one-off reads dispatch needs) run as `tea.Cmd`s through `internal/actions`. Routing them through the poller would mean a request queue for no benefit. The poller is told to refresh afterwards.
- TDD: failing test, implement, pass, commit. `make build`, `make test`, `make lint` green before each commit. Trunk-based on `main`.

## Review Focus

1. `r` on a run that is still active, or `x` on one that has completed, must explain why and make no API call. (Task 6)
2. Pressing `y` twice quickly, or `r` then `y` `y`, must send exactly one request. (Task 5)
3. Workflow files whose trigger is written `on: workflow_dispatch`, `on: [push, workflow_dispatch]`, `on: {workflow_dispatch: }` (null), or whose `on` key YAML decodes as boolean `true`, must all be recognised as dispatchable. (Task 3)
4. A 403 or 404 on a write (no write access, missing `workflow` scope, Actions disabled) must show a permission message in the status bar. (Task 1)
5. A required input left empty, or a choice default that is not among the options, must block submission with a message naming the input. (Task 9)

---

### Task 1: Write calls in the client

**Files:** `internal/gh/client.go`, `internal/gh/actions.go`, `internal/gh/actions_test.go`, `internal/gh/errors.go`

**Produces:** `func (c *Client) post(ctx, path string, body any) error` (2xx success; errors through `checkStatus`); `RerunRun(ctx, owner, repo string, runID int64) error` (`POST /repos/{o}/{r}/actions/runs/{id}/rerun`); `RerunFailedJobs(ctx, owner, repo string, runID int64) error` (`.../rerun-failed-jobs`); `CancelRun(ctx, owner, repo string, runID int64) error` (`.../cancel`); `DispatchWorkflow(ctx, owner, repo string, workflowID int64, ref string, inputs map[string]string) error` (`POST .../actions/workflows/{id}/dispatches` with `{"ref":…,"inputs":{…}}`, inputs omitted when empty); `type PermissionError struct{ Status int; Message string }`.

- [ ] Tests: each call uses method POST, the right path, the auth headers, and JSON body where relevant; 201, 202 and 204 are success; a 403 or 404 on a write returns `*PermissionError` whose message mentions write access and the `workflow` scope (Review Focus 4); a 409 (for example "Cannot cancel a workflow run that is completed") returns `*APIError` with GitHub's message; the rate-limit header is recorded as for GETs.
- [ ] Implement.
- [ ] Commit.

### Task 2: Workflows and workflow files

**Files:** `internal/gh/workflows.go`, `internal/gh/workflows_test.go`, `internal/gh/types.go`, `internal/gh/testdata/workflows.json`, `internal/gh/testdata/contents.json`

**Produces:** `Repo.DefaultBranch string` (decoded from `default_branch`); `type Workflow struct{ ID int64; Name, Path, State string }`; `ListWorkflows(ctx, owner, repo string) ([]Workflow, error)` (paginates); `WorkflowFile(ctx, owner, repo, path, ref string) ([]byte, error)` via `GET /repos/{o}/{r}/contents/{path}?ref=` decoding base64.

- [ ] Tests: `default_branch` decoded by `ListRepos` and `GetRepo`; `ListWorkflows` decodes and follows `Link` pagination; `WorkflowFile` decodes base64 with embedded newlines, sends `ref` when set and omits it when empty, and returns 404 as `*APIError`.
- [ ] Implement.
- [ ] Commit.

### Task 3: Parse workflow_dispatch inputs

**Files:** `internal/workflow/dispatch.go`, `internal/workflow/dispatch_test.go`, `internal/workflow/testdata/*.yml`

**Produces:** `type Input struct{ Name, Description, Type, Default string; Required bool; Options []string }` (`Type` one of `string`, `boolean`, `choice`, `number`, `environment`; empty means `string`); `func ParseDispatch(file []byte) (dispatchable bool, inputs []Input, err error)`; inputs returned in file order.

- [ ] Tests: `on: workflow_dispatch` string; `on: [push, workflow_dispatch]` list; `on: {workflow_dispatch: null}`; map with inputs of every type including choice options and boolean default `true` (YAML bool) rendered as `"true"`; numeric default rendered as its text; the `on` key decoded as boolean `true` (YAML 1.1 style) is still found (Review Focus 3); no dispatch trigger → `false`; invalid YAML → error; input order preserved.
- [ ] Implement by decoding into `yaml.Node` (keeps order and handles the `on`/`true` key).
- [ ] Commit.

### Task 4: Actions service

**Files:** `internal/actions/actions.go`, `internal/actions/actions_test.go`

**Consumes:** Tasks 1–3.

**Produces:** `type API interface{ RerunRun; RerunFailedJobs; CancelRun; DispatchWorkflow; ListJobs; ListWorkflows; WorkflowFile }`; `type Service struct`; `New(api API) *Service`; `(*Service) Rerun(ctx, run gh.Run) (string, error)` which reruns failed jobs if any job's conclusion is failure/timed_out/cancelled, else all, and returns "rerun requested" or "rerun of failed jobs requested"; `(*Service) Cancel(ctx, run gh.Run) (string, error)`; `type Dispatchable struct{ Workflow gh.Workflow; Inputs []workflow.Input }`; `(*Service) Dispatchable(ctx, owner, repo, ref string) ([]Dispatchable, error)` listing active workflows whose file at ref has a dispatch trigger; `(*Service) Dispatch(ctx, owner, repo string, wf gh.Workflow, ref string, inputs []workflow.Input, values map[string]string) (string, error)`; `func Validate(inputs []workflow.Input, values map[string]string) error`.

- [ ] Tests with a fake API: Rerun picks failed-only when any job failed and all otherwise; Rerun and Cancel refuse locally with a clear error (no API call) when the run is active or completed respectively (Review Focus 1, logic side); `Dispatchable` skips disabled workflows and files without a trigger, and keeps going when one file fails to load; `Validate` rejects a missing required input, a boolean other than `true`/`false`, a choice value not among options, and a number that does not parse, naming the input (Review Focus 5, logic side); `Dispatch` sends only non-empty values plus required ones.
- [ ] Implement.
- [ ] Commit.

### Task 5: Confirmation prompt

**Files:** `internal/tui/confirm.go`, `internal/tui/confirm_test.go`, `internal/tui/model.go`, `internal/tui/statusbar.go`

**Produces:** message `Confirm{Prompt string; Run tea.Cmd}`; screens return `ask(prompt, cmd)` to request confirmation; message `ActionResult{Text string; Err error}`.

- [ ] Tests: `Confirm` shows "<prompt> [y/N]" in the status bar; `y` runs the command once and clears the prompt; `y` again does nothing (Review Focus 2); `n`, `esc` or any other key cancels with "cancelled" and the key is not passed to the screen; while a prompt is open, `q` cancels the prompt rather than quitting; `ActionResult` success flashes its text for 5 s; an error shows in the status bar's error slot.
- [ ] Implement in the root model.
- [ ] Commit.

### Task 6: Rerun and cancel keys

**Files:** `internal/tui/operate.go`, `internal/tui/operate_test.go`, `internal/tui/model.go`, `internal/tui/runs.go`, `internal/tui/jobs.go`, `internal/tui/tailview.go`, `internal/tui/helpers_test.go`

**Consumes:** `Context.Actions` (new field), interface `Actions{ Rerun(ctx, gh.Run) (string, error); Cancel(ctx, gh.Run) (string, error); Dispatchable(...); Dispatch(...) }` satisfied by `*actions.Service`.

- [ ] Fake `Actions` in tests recording calls.
- [ ] Tests: on Runs (selected run), Jobs and Tail (their run) `r` asks "Rerun <repo> #<n> <workflow>?" and on `y` calls `Rerun` once; `x` asks "Cancel …?" and calls `Cancel`; `r` on an active run shows "run is still in progress" and asks nothing; `x` on a completed run shows "run already finished" and asks nothing (Review Focus 1); on success the result text is flashed and `Refresh("runs:<repo>")` plus `Refresh("jobs:<id>")` are called; on error the message shows and nothing is refreshed; the screen uses the latest run status from the store, not the one captured when the screen opened.
- [ ] Implement a shared helper used by the three screens.
- [ ] Commit.

### Task 7: Open in browser

**Files:** `internal/tui/operate.go`, `internal/tui/operate_test.go`

**Consumes:** `Context.Open func(url string) error` (new field).

- [ ] Tests: `o` on Board opens `https://github.com/<owner>/<repo>/actions`; on Runs opens the run's `HTMLURL`; on Jobs and Tail opens the job's `HTMLURL`, falling back to the run's; an `Open` error shows in the status bar; an empty list opens nothing.
- [ ] Implement.
- [ ] Commit.

### Task 8: Dispatch workflow picker

**Files:** `internal/tui/dispatch.go`, `internal/tui/dispatch_test.go`

- [ ] Tests: `d` on Board, Runs, Jobs or Tail pushes the picker for that repo; while loading it shows "loading workflows…" (the load is a `tea.Cmd` calling `Dispatchable`); then it lists dispatchable workflows by name and path, with the cursor on the selected run's workflow when there is one; no dispatchable workflows shows "no workflows with a workflow_dispatch trigger"; a load error shows in the body; `enter` pushes the dispatch form for the highlighted workflow; `esc` pops.
- [ ] Ref used for loading: the selected run's branch, else `Repo.DefaultBranch` from the store, else `main`.
- [ ] Implement.
- [ ] Commit.

### Task 9: Dispatch form

**Files:** `internal/tui/dispatchform.go`, `internal/tui/dispatchform_test.go`

- [ ] Tests (golden plus behaviour): first field is the ref, pre-filled; then one field per input with its description and pre-filled default; `tab`/`down`/`enter` move to the next field and `shift+tab`/`up` back; boolean fields toggle with `space` between `true` and `false`; choice fields cycle with `space`/`←`/`→` through options; text and number fields edit with Bubbles `textinput`; `enter` on the last field runs `Validate` and, on failure, shows the message naming the input and stays (Review Focus 5); on success it asks "Dispatch <workflow> on <ref>?" through `Confirm`; `y` calls `Dispatch` once, pops back to where `d` was pressed, flashes "dispatch requested" and refreshes the repo's runs; `esc` cancels; the form fits 40×10 and scrolls fields when they do not fit.
- [ ] Implement.
- [ ] Commit.

### Task 10: Wire up, help, README, verification

**Files:** `cmd/ghtui/tui.go`, `cmd/ghtui/deps.go`, `internal/tui/help.go`, `README.md`

- [ ] `runTUI` builds `actions.New(client)` and an opener (`open` on macOS, `xdg-open` elsewhere, via `deps.run`) and passes both in `tui.Context`.
- [ ] Help lists `r` `x` `d` `o` and the y/n confirmation; README key table updated.
- [ ] Test: the opener runs `open <url>` on darwin and `xdg-open <url>` elsewhere (injected GOOS).
- [ ] Manual: `o` on each screen; `d` picker and form against a real repo with a dispatchable workflow, confirmed with `n` (nothing sent). Running a real rerun, cancel or dispatch is an outward action: ask the user which repo and run to use, and only then confirm with `y`.
- [ ] Commit.

## Self-review notes

- Spec §6 keys `r` `x` `d` `o` and confirmation: Tasks 5–9. §9 "Rerun/cancel/dispatch fails → error in status bar, no local state change": Tasks 5–6, 9. §1 "Operate workflows": Tasks 4, 8, 9.
- Names used across tasks: `PermissionError`, `Workflow`, `WorkflowFile`, `ParseDispatch`, `Input`, `Service`, `Dispatchable`, `Validate`, `Confirm`, `ActionResult`, `Context.Actions`, `Context.Open`. Consistent.
