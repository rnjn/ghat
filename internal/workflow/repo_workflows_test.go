package workflow

import (
	"os"
	"testing"
)

// The repo's own workflows must parse the way ghtui expects.
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
