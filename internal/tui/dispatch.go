package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghtui/internal/actions"
)

// dispatchLoaded carries the dispatchable workflows of a repo.
type dispatchLoaded struct {
	picker *dispatchPicker
	list   []actions.Dispatchable
	err    error
}

// dispatchPicker lists the repo's workflows that can be triggered by hand.
type dispatchPicker struct {
	repoKey string
	ref     string
	prefer  int64 // workflow ID to preselect
	loading bool
	list    []actions.Dispatchable
	err     error
	cur     cursor
}

// dispatchKey opens the picker for repoKey when k is d. ref and prefer come
// from the current run, if any.
func dispatchKey(ctx *Context, k, repoKey, ref string, prefer int64) tea.Cmd {
	if k != "d" {
		return nil
	}
	if ref == "" {
		ref = "main"
		if rs, ok := ctx.Store.Repo(repoKey); ok && rs.Repo.DefaultBranch != "" {
			ref = rs.Repo.DefaultBranch
		}
	}
	p := &dispatchPicker{repoKey: repoKey, ref: ref, prefer: prefer, loading: true}
	acts := ctx.Actions
	owner, repo := splitKey(repoKey)
	load := func() tea.Msg {
		list, err := acts.Dispatchable(context.Background(), owner, repo, ref)
		return dispatchLoaded{picker: p, list: list, err: err}
	}
	return tea.Batch(push(p), load)
}

// dispatchForRun opens the picker for a run's repo, branch and workflow.
func dispatchForRun(ctx *Context, k string, runID int64) tea.Cmd {
	if k != "d" {
		return nil
	}
	run, ok := ctx.Store.Run(runID)
	if !ok {
		return nil
	}
	return dispatchKey(ctx, k, run.RepoKey, run.Branch, run.WorkflowID)
}

func (p *dispatchPicker) Title() string { return "Dispatch · " + p.repoKey }

func (p *dispatchPicker) Update(msg tea.Msg, ctx *Context) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case dispatchLoaded:
		if m.picker != p { // a load started by an earlier picker
			return p, nil
		}
		p.loading, p.list, p.err = false, m.list, m.err
		for i, d := range p.list {
			if d.Workflow.ID == p.prefer {
				p.cur.pos = i
			}
		}
	case tea.KeyPressMsg:
		if p.cur.navKey(m.String(), len(p.list), 10) {
			return p, nil
		}
		if m.String() == "enter" && len(p.list) > 0 {
			p.cur.clamp(len(p.list))
			d := p.list[p.cur.pos]
			if d.Err != nil {
				return p, fail(fmt.Errorf("%s cannot be dispatched: %w", d.Workflow.Path, d.Err))
			}
			return p, push(newDispatchForm(p.repoKey, p.ref, d))
		}
	}
	return p, nil
}

func (p *dispatchPicker) View(ctx *Context, width, height int) string {
	switch {
	case p.loading:
		return fit([]string{"", "  loading workflows…"}, width, height)
	case p.err != nil:
		return fit([]string{"", "  " + styleError.Render(p.err.Error())}, width, height)
	case len(p.list) == 0:
		return fit([]string{"", "  no workflows with a workflow_dispatch trigger"}, width, height)
	}
	head := []string{fmt.Sprintf("  ref %s · enter to fill inputs", p.ref), ""}
	start, end := p.cur.window(len(p.list), height-len(head)-1)
	var rows [][]string
	for _, d := range p.list[start:end] {
		if d.Err != nil {
			rows = append(rows, []string{styleDim.Render(d.Workflow.Name), styleDim.Render(d.Workflow.Path), styleError.Render("error: " + d.Err.Error())})
			continue
		}
		rows = append(rows, []string{d.Workflow.Name, styleDim.Render(d.Workflow.Path), fmt.Sprintf("%d inputs", len(d.Inputs))})
	}
	return fit(append(head, table([]string{"WORKFLOW", "FILE", ""}, rows, p.cur.pos-start)...), width, height)
}

func splitKey(key string) (owner, repo string) {
	owner, repo, _ = strings.Cut(key, "/")
	return owner, repo
}
