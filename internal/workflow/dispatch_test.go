package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDispatchTriggerForms(t *testing.T) {
	for file, want := range map[string]bool{
		"scalar.yml": true, "list.yml": true, "nullmap.yml": true, "quoted.yml": true, "inputs.yml": true, "none.yml": false,
	} {
		ok, _, err := ParseDispatch(load(t, file))
		if err != nil || ok != want {
			t.Errorf("%s: dispatchable=%v err=%v, want %v", file, ok, err, want)
		}
	}
}

func TestBoolKeyOn(t *testing.T) {
	// YAML 1.1 parsers read a bare `on` key as boolean true; some tools
	// rewrite it that way.
	ok, _, err := ParseDispatch([]byte("true:\n  workflow_dispatch:\n"))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestDispatchInputs(t *testing.T) {
	_, in, err := ParseDispatch(load(t, "inputs.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(in) != 5 {
		t.Fatalf("inputs = %+v", in)
	}
	names := []string{"environment", "dry_run", "replicas", "version", "target"}
	for i, n := range names {
		if in[i].Name != n {
			t.Fatalf("order: got %s at %d, want %s", in[i].Name, i, n)
		}
	}
	env := in[0]
	if env.Type != "choice" || !env.Required || env.Default != "staging" || env.Description != "Where to deploy" ||
		len(env.Options) != 2 || env.Options[1] != "production" {
		t.Fatalf("environment = %+v", env)
	}
	if in[1].Type != "boolean" || in[1].Default != "true" {
		t.Fatalf("dry_run = %+v", in[1])
	}
	if in[2].Type != "number" || in[2].Default != "3" {
		t.Fatalf("replicas = %+v", in[2])
	}
	if in[3].Type != "string" || in[3].Required {
		t.Fatalf("version = %+v (empty type means string)", in[3])
	}
	if in[4].Type != "environment" {
		t.Fatalf("target = %+v", in[4])
	}
}

func TestDispatchInvalidYAML(t *testing.T) {
	if _, _, err := ParseDispatch([]byte("on: [unclosed")); err == nil {
		t.Fatal("want error")
	}
}
