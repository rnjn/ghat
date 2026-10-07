package actions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ghtui/internal/gh"
	"ghtui/internal/workflow"
)

type fakeAPI struct {
	fmu       sync.Mutex
	calls     []string
	jobs      []gh.Job
	workflows []gh.Workflow
	files     map[string]string
	fileErr   map[string]error
	dispatch  map[string]string
}

func (f *fakeAPI) RerunRun(_ context.Context, o, r string, id int64) error {
	f.calls = append(f.calls, fmt.Sprintf("rerun %d", id))
	return nil
}
func (f *fakeAPI) RerunFailedJobs(_ context.Context, o, r string, id int64) error {
	f.calls = append(f.calls, fmt.Sprintf("rerun-failed %d", id))
	return nil
}
func (f *fakeAPI) CancelRun(_ context.Context, o, r string, id int64) error {
	f.calls = append(f.calls, fmt.Sprintf("cancel %d", id))
	return nil
}
func (f *fakeAPI) DispatchWorkflow(_ context.Context, o, r string, id int64, ref string, in map[string]string) error {
	f.calls = append(f.calls, fmt.Sprintf("dispatch %d %s", id, ref))
	f.dispatch = in
	return nil
}
func (f *fakeAPI) ListJobs(_ context.Context, o, r string, id int64) ([]gh.Job, error) {
	f.calls = append(f.calls, "list-jobs")
	return f.jobs, nil
}
func (f *fakeAPI) ListWorkflows(_ context.Context, o, r string) ([]gh.Workflow, error) {
	return f.workflows, nil
}
func (f *fakeAPI) WorkflowFile(_ context.Context, o, r, path, ref string) ([]byte, error) {
	f.fmu.Lock()
	defer f.fmu.Unlock()
	f.calls = append(f.calls, "file "+path+"@"+ref)
	if err := f.fileErr[path]; err != nil {
		return nil, err
	}
	return []byte(f.files[path]), nil
}

func run(status, conclusion string) gh.Run {
	return gh.Run{ID: 9, RepoKey: "o/r", Number: 3, Status: status, Conclusion: conclusion}
}

func TestRerunFailedOnlyWhenAJobFailed(t *testing.T) {
	for conclusion, want := range map[string]string{"failure": "rerun-failed 9", "timed_out": "rerun-failed 9", "cancelled": "rerun-failed 9", "success": "rerun 9"} {
		api := &fakeAPI{jobs: []gh.Job{{Conclusion: "success"}, {Conclusion: conclusion}}}
		msg, err := New(api).Rerun(context.Background(), run("completed", "failure"))
		if err != nil || api.calls[len(api.calls)-1] != want {
			t.Fatalf("%s: calls %v err %v", conclusion, api.calls, err)
		}
		if !strings.Contains(msg, "requested") {
			t.Fatalf("msg %q", msg)
		}
	}
}

func TestRerunAndCancelRefuseLocally(t *testing.T) {
	api := &fakeAPI{}
	s := New(api)
	if _, err := s.Rerun(context.Background(), run("in_progress", "")); err == nil || !strings.Contains(err.Error(), "still in progress") {
		t.Fatalf("rerun active: %v", err)
	}
	if _, err := s.Cancel(context.Background(), run("completed", "success")); err == nil || !strings.Contains(err.Error(), "already finished") {
		t.Fatalf("cancel completed: %v", err)
	}
	if len(api.calls) != 0 {
		t.Fatalf("API called: %v", api.calls)
	}
	if _, err := s.Cancel(context.Background(), run("queued", "")); err != nil || api.calls[0] != "cancel 9" {
		t.Fatalf("cancel queued: %v %v", api.calls, err)
	}
}

const dispatchYAML = "on:\n  workflow_dispatch:\n    inputs:\n      env:\n        required: true\n"

func TestDispatchable(t *testing.T) {
	api := &fakeAPI{
		workflows: []gh.Workflow{
			{ID: 1, Name: "Deploy", Path: "deploy.yml", State: "active"},
			{ID: 2, Name: "CI", Path: "ci.yml", State: "active"},
			{ID: 3, Name: "Off", Path: "off.yml", State: "disabled_manually"},
			{ID: 4, Name: "Broken", Path: "broken.yml", State: "active"},
			{ID: 5, Name: "Manual", Path: "manual.yml", State: "active"},
		},
		files:   map[string]string{"deploy.yml": dispatchYAML, "ci.yml": "on: push\n", "off.yml": dispatchYAML, "manual.yml": "on: workflow_dispatch\n"},
		fileErr: map[string]error{"broken.yml": &gh.APIError{Status: 404}},
	}
	got, err := New(api).Dispatchable(context.Background(), "o", "r", "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Workflow.Name != "Deploy" || got[1].Workflow.Name != "Manual" || len(got[0].Inputs) != 1 {
		t.Fatalf("got %+v", got)
	}
	for _, c := range api.calls {
		if c == "file off.yml@main" {
			t.Fatal("fetched a disabled workflow's file")
		}
	}
}

func TestDispatchableAllFilesFail(t *testing.T) {
	api := &fakeAPI{workflows: []gh.Workflow{{ID: 1, Path: "a.yml", State: "active"}},
		fileErr: map[string]error{"a.yml": errors.New("boom")}}
	if _, err := New(api).Dispatchable(context.Background(), "o", "r", "main"); err == nil {
		t.Fatal("want error when no workflow file could be read")
	}
}

func TestValidate(t *testing.T) {
	inputs := []workflow.Input{
		{Name: "env", Type: "choice", Options: []string{"staging", "prod"}, Required: true},
		{Name: "dry", Type: "boolean"},
		{Name: "n", Type: "number"},
		{Name: "tag", Type: "string", Required: true},
	}
	ok := map[string]string{"env": "prod", "dry": "false", "n": "2.5", "tag": "v1"}
	if err := Validate(inputs, ok); err != nil {
		t.Fatalf("valid values rejected: %v", err)
	}
	for name, bad := range map[string]map[string]string{
		"tag": {"env": "prod", "dry": "true", "n": "1", "tag": ""},
		"env": {"env": "qa", "dry": "true", "n": "1", "tag": "v1"},
		"dry": {"env": "prod", "dry": "yes", "n": "1", "tag": "v1"},
		"n":   {"env": "prod", "dry": "true", "n": "three", "tag": "v1"},
	} {
		err := Validate(inputs, bad)
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: err = %v, want it named", name, err)
		}
	}
}

func TestDispatchSendsValues(t *testing.T) {
	api := &fakeAPI{}
	inputs := []workflow.Input{{Name: "env", Required: true, Type: "string"}, {Name: "note", Type: "string"}, {Name: "tag", Type: "string"}}
	msg, err := New(api).Dispatch(context.Background(), "o", "r", gh.Workflow{ID: 7, Name: "Deploy"}, "main", inputs,
		map[string]string{"env": "prod", "note": "", "tag": "v2"})
	if err != nil || msg != "dispatch of Deploy on main requested" {
		t.Fatalf("msg %q err %v", msg, err)
	}
	if api.calls[0] != "dispatch 7 main" || len(api.dispatch) != 2 || api.dispatch["tag"] != "v2" {
		t.Fatalf("calls %v inputs %v", api.calls, api.dispatch)
	}
	if _, err := New(api).Dispatch(context.Background(), "o", "r", gh.Workflow{ID: 7}, "main", inputs, map[string]string{}); err == nil {
		t.Fatal("dispatch with missing required input not rejected")
	}
	if _, err := New(api).Dispatch(context.Background(), "o", "r", gh.Workflow{ID: 7}, " ", nil, nil); err == nil {
		t.Fatal("empty ref not rejected")
	}
}

// slowFiles answers WorkflowFile after a delay; safe for concurrent calls.
type slowFiles struct {
	fakeAPI
	mu       sync.Mutex
	inFlight int
	peak     int
}

func (s *slowFiles) WorkflowFile(_ context.Context, o, r, path, ref string) ([]byte, error) {
	s.mu.Lock()
	s.inFlight++
	s.peak = max(s.peak, s.inFlight)
	s.mu.Unlock()
	time.Sleep(30 * time.Millisecond)
	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()
	return []byte("on: workflow_dispatch\n"), nil
}

func TestDispatchableFetchesFilesConcurrentlyInOrder(t *testing.T) {
	api := &slowFiles{}
	for i := 1; i <= 24; i++ {
		api.workflows = append(api.workflows, gh.Workflow{ID: int64(i), Name: fmt.Sprintf("wf%02d", i), Path: fmt.Sprintf("%02d.yml", i), State: "active"})
	}
	start := time.Now()
	got, err := New(api).Dispatchable(context.Background(), "o", "r", "main")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 300*time.Millisecond {
		t.Fatalf("took %v for 24 files (sequential would be ~720ms)", d)
	}
	if api.peak > maxFetch {
		t.Fatalf("peak concurrency %d, limit %d", api.peak, maxFetch)
	}
	for i, d := range got {
		if d.Workflow.ID != int64(i+1) {
			t.Fatalf("order broken at %d: %v", i, d.Workflow.Name)
		}
	}
}

func TestValidateNumbersStrictly(t *testing.T) {
	in := []workflow.Input{{Name: "n", Type: "number"}}
	for _, v := range []string{"3", "-2", "2.5", "0", "-0.75"} {
		if err := Validate(in, map[string]string{"n": v}); err != nil {
			t.Errorf("%q rejected: %v", v, err)
		}
	}
	for _, v := range []string{"NaN", "Inf", "-inf", "0x1p3", "1e5", "1_000", ".", "1."} {
		if err := Validate(in, map[string]string{"n": v}); err == nil {
			t.Errorf("%q accepted", v)
		}
	}
}
