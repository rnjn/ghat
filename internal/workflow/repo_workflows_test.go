package workflow

import (
	"os"
	"testing"
)

// The repo's own workflows must parse the way ghat expects.
func TestRepoWorkflowsAreDispatchable(t *testing.T) {
	for file, inputs := range map[string]int{"../../.github/workflows/dogfood.yml": 3, "../../.github/workflows/ci.yml": 0} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		ok, in, err := ParseDispatch(b)
		if err != nil || !ok || len(in) != inputs {
			t.Fatalf("%s: dispatchable=%v inputs=%d err=%v", file, ok, len(in), err)
		}
	}
}

// The dogfood workflow has a fan-in so the graph view has edges to draw.
func TestDogfoodHasReportStage(t *testing.T) {
	b, err := os.ReadFile("../../.github/workflows/dogfood.yml")
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := ParseJobs(b)
	if err != nil || len(jobs) != 3 || jobs[2].Key != "report" || len(jobs[2].Needs) != 2 {
		t.Fatalf("jobs %+v err %v", jobs, err)
	}
}
