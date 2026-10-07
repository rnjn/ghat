// Package gh is a thin typed client for the GitHub Actions REST API.
package gh

import (
	"errors"
	"strings"
)

// ErrNoAuth means no token could be found in the environment or the gh CLI.
var ErrNoAuth = errors.New("no GitHub token found: set GH_TOKEN or run `gh auth login`")

// ResolveToken finds a token in GH_TOKEN, then GITHUB_TOKEN, then the output
// of `gh auth token`. env and run are injected so callers can test it.
func ResolveToken(env func(string) string, run func(name string, args ...string) ([]byte, error)) (string, error) {
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(env(k)); v != "" {
			return v, nil
		}
	}
	out, err := run("gh", "auth", "token")
	if err != nil {
		return "", ErrNoAuth
	}
	if tok := strings.TrimSpace(string(out)); tok != "" {
		return tok, nil
	}
	return "", ErrNoAuth
}
