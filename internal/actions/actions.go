// Package actions performs the user-triggered operations: rerun, cancel
// and workflow dispatch. Each returns a short "… requested" message.
package actions

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/rnjn/ghtui/internal/gh"
	"github.com/rnjn/ghtui/internal/store"
	"github.com/rnjn/ghtui/internal/workflow"
)

// API is the subset of *gh.Client the actions need.
type API interface {
	RerunRun(ctx context.Context, owner, repo string, runID int64) error
	RerunFailedJobs(ctx context.Context, owner, repo string, runID int64) error
	CancelRun(ctx context.Context, owner, repo string, runID int64) error
	DispatchWorkflow(ctx context.Context, owner, repo string, workflowID int64, ref string, inputs map[string]string) error
	ListJobs(ctx context.Context, owner, repo string, runID int64) ([]gh.Job, error)
	ListWorkflows(ctx context.Context, owner, repo string) ([]gh.Workflow, error)
	WorkflowFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error)
}

// Service performs actions through the API.
type Service struct{ api API }

// New returns a Service.
func New(api API) *Service { return &Service{api: api} }

func split(repoKey string) (owner, repo string) {
	owner, repo, _ = strings.Cut(repoKey, "/")
	return owner, repo
}

// Rerun reruns the failed jobs of a finished run if any failed, else all.
func (s *Service) Rerun(ctx context.Context, run gh.Run) (string, error) {
	if store.IsActive(run.Status) {
		return "", errors.New("run is still in progress")
	}
	owner, repo := split(run.RepoKey)
	jobs, err := s.api.ListJobs(ctx, owner, repo, run.ID)
	if err != nil {
		return "", err
	}
	for _, j := range jobs {
		if j.Conclusion == "failure" || j.Conclusion == "timed_out" || j.Conclusion == "cancelled" {
			if err := s.api.RerunFailedJobs(ctx, owner, repo, run.ID); err != nil {
				return "", err
			}
			return fmt.Sprintf("rerun of failed jobs of #%d requested", run.Number), nil
		}
	}
	if err := s.api.RerunRun(ctx, owner, repo, run.ID); err != nil {
		return "", err
	}
	return fmt.Sprintf("rerun of #%d requested", run.Number), nil
}

// Cancel cancels a run that has not finished.
func (s *Service) Cancel(ctx context.Context, run gh.Run) (string, error) {
	if run.Status == "completed" {
		return "", errors.New("run already finished")
	}
	owner, repo := split(run.RepoKey)
	if err := s.api.CancelRun(ctx, owner, repo, run.ID); err != nil {
		return "", err
	}
	return fmt.Sprintf("cancel of #%d requested", run.Number), nil
}

// Dispatchable is a workflow that can be triggered by hand, with its inputs.
type Dispatchable struct {
	Workflow gh.Workflow
	Inputs   []workflow.Input
	Err      error // the workflow file could not be parsed
}

// maxFetch bounds concurrent workflow-file requests.
const maxFetch = 8

// Dispatchable lists active workflows whose file at ref has a
// workflow_dispatch trigger. Unreadable files are skipped; it fails only
// if none could be read.
func (s *Service) Dispatchable(ctx context.Context, owner, repo, ref string) ([]Dispatchable, error) {
	wfs, err := s.api.ListWorkflows(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	type result struct {
		file []byte
		err  error
	}
	results := make([]result, len(wfs))
	sem := make(chan struct{}, maxFetch)
	var wg sync.WaitGroup
	for i, wf := range wfs {
		if wf.State != "active" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			file, err := s.api.WorkflowFile(ctx, owner, repo, wf.Path, ref)
			results[i] = result{file: file, err: err}
		}()
	}
	wg.Wait()

	var out []Dispatchable
	var firstErr error
	read := 0
	for i, wf := range wfs {
		if wf.State != "active" {
			continue
		}
		if err := results[i].err; err != nil {
			firstErr = cmpErr(firstErr, err)
			continue
		}
		read++
		ok, inputs, err := workflow.ParseDispatch(results[i].file)
		if err != nil {
			out = append(out, Dispatchable{Workflow: wf, Err: err})
			continue
		}
		if !ok {
			continue
		}
		out = append(out, Dispatchable{Workflow: wf, Inputs: inputs})
	}
	if read == 0 && firstErr != nil {
		return nil, fmt.Errorf("could not read any workflow file: %w", firstErr)
	}
	return out, nil
}

func cmpErr(first, err error) error {
	if first != nil {
		return first
	}
	return err
}

// numberRe is a plain decimal: what GitHub's number inputs accept.
var numberRe = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

// Validate checks values against the inputs' types and required flags.
func Validate(inputs []workflow.Input, values map[string]string) error {
	for _, in := range inputs {
		v := strings.TrimSpace(values[in.Name])
		if v == "" {
			if in.Required {
				return fmt.Errorf("input %q is required", in.Name)
			}
			continue
		}
		switch in.Type {
		case "boolean":
			if v != "true" && v != "false" {
				return fmt.Errorf("input %q must be true or false", in.Name)
			}
		case "choice":
			if !slices.Contains(in.Options, v) {
				return fmt.Errorf("input %q must be one of %s", in.Name, strings.Join(in.Options, ", "))
			}
		case "number":
			if !numberRe.MatchString(v) {
				return fmt.Errorf("input %q must be a number", in.Name)
			}
		}
	}
	return nil
}

// Dispatch validates values and triggers wf on ref. Empty optional values
// are left out so the workflow's own defaults apply.
func (s *Service) Dispatch(ctx context.Context, owner, repo string, wf gh.Workflow, ref string, inputs []workflow.Input, values map[string]string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("ref is required")
	}
	if err := Validate(inputs, values); err != nil {
		return "", err
	}
	send := map[string]string{}
	for _, in := range inputs {
		if v := strings.TrimSpace(values[in.Name]); v != "" || in.Required {
			send[in.Name] = v
		}
	}
	if err := s.api.DispatchWorkflow(ctx, owner, repo, wf.ID, ref, send); err != nil {
		return "", err
	}
	return fmt.Sprintf("dispatch of %s on %s requested", wf.Name, ref), nil
}
