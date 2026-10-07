package gh

import (
	"errors"
	"strings"
	"testing"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func runReturning(out string, err error) func(string, ...string) ([]byte, error) {
	return func(name string, args ...string) ([]byte, error) {
		if name != "gh" || strings.Join(args, " ") != "auth token" {
			return nil, errors.New("unexpected command")
		}
		return []byte(out), err
	}
}

func runMustNotBeCalled(t *testing.T) func(string, ...string) ([]byte, error) {
	return func(string, ...string) ([]byte, error) {
		t.Fatal("gh should not be invoked")
		return nil, nil
	}
}

func TestResolveTokenPrefersGHToken(t *testing.T) {
	tok, err := ResolveToken(envMap(map[string]string{"GH_TOKEN": "a", "GITHUB_TOKEN": "b"}), runMustNotBeCalled(t))
	if err != nil || tok != "a" {
		t.Fatalf("got %q, %v; want a", tok, err)
	}
}

func TestResolveTokenFallsBackToGitHubToken(t *testing.T) {
	tok, err := ResolveToken(envMap(map[string]string{"GITHUB_TOKEN": "b"}), runMustNotBeCalled(t))
	if err != nil || tok != "b" {
		t.Fatalf("got %q, %v; want b", tok, err)
	}
}

func TestResolveTokenFallsBackToGhCLI(t *testing.T) {
	tok, err := ResolveToken(envMap(nil), runReturning("gho_xyz\n", nil))
	if err != nil || tok != "gho_xyz" {
		t.Fatalf("got %q, %v; want gho_xyz", tok, err)
	}
}

func TestResolveTokenNoAuth(t *testing.T) {
	for name, run := range map[string]func(string, ...string) ([]byte, error){
		"gh fails": runReturning("", errors.New("exit 1")),
		"gh empty": runReturning("  \n", nil),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ResolveToken(envMap(nil), run)
			if !errors.Is(err, ErrNoAuth) {
				t.Fatalf("err = %v, want ErrNoAuth", err)
			}
			if !strings.Contains(err.Error(), "gh auth login") {
				t.Fatalf("message %q does not mention gh auth login", err)
			}
		})
	}
}
