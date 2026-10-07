package config

import (
	"errors"
	"strings"
	"testing"
)

func TestInferRepo(t *testing.T) {
	for _, u := range []string{
		"git@github.com:o/r.git",
		"git@github.com:o/r",
		"ssh://git@github.com/o/r.git",
		"https://github.com/o/r",
		"https://github.com/o/r.git",
		"https://github.com/o/r/",
	} {
		owner, repo, err := InferRepo(u)
		if err != nil || owner != "o" || repo != "r" {
			t.Errorf("InferRepo(%q) = %q, %q, %v", u, owner, repo, err)
		}
	}
}

func TestInferRepoRejects(t *testing.T) {
	for _, u := range []string{"https://gitlab.com/o/r.git", "git@example.com:o/r.git", "https://github.com/o", ""} {
		if _, _, err := InferRepo(u); err == nil {
			t.Errorf("InferRepo(%q) succeeded", u)
		}
	}
}

func TestCurrentRepo(t *testing.T) {
	run := func(name string, args ...string) ([]byte, error) {
		if name != "git" || strings.Join(args, " ") != "remote get-url origin" {
			return nil, errors.New("unexpected command")
		}
		return []byte("git@github.com:acme/api.git\n"), nil
	}
	owner, repo, err := CurrentRepo(run)
	if err != nil || owner != "acme" || repo != "api" {
		t.Fatalf("got %q %q %v", owner, repo, err)
	}
}

func TestCurrentRepoNoGit(t *testing.T) {
	run := func(string, ...string) ([]byte, error) { return nil, errors.New("not a git repository") }
	if _, _, err := CurrentRepo(run); err == nil || !strings.Contains(err.Error(), "owner/repo") {
		t.Fatalf("err = %v, want hint to pass owner/repo", err)
	}
}

func TestParseRepo(t *testing.T) {
	if o, r, err := ParseRepo("acme/api"); err != nil || o != "acme" || r != "api" {
		t.Fatalf("got %q %q %v", o, r, err)
	}
	for _, bad := range []string{"acme", "acme/", "/api", "a/b/c"} {
		if _, _, err := ParseRepo(bad); err == nil {
			t.Errorf("ParseRepo(%q) succeeded", bad)
		}
	}
}
