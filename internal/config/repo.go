package config

import (
	"fmt"
	"strings"
)

// InferRepo extracts owner and repo from a github.com remote URL in SSH,
// ssh:// or HTTPS form.
func InferRepo(remoteURL string) (owner, repo string, err error) {
	u := strings.TrimSpace(remoteURL)
	var path string
	for _, prefix := range []string{"git@github.com:", "ssh://git@github.com/", "https://github.com/", "http://github.com/"} {
		if rest, ok := strings.CutPrefix(u, prefix); ok {
			path = rest
			break
		}
	}
	if path == "" {
		return "", "", fmt.Errorf("remote %q is not a github.com repository", remoteURL)
	}
	path = strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git")
	return ParseRepo(path)
}

// ParseRepo splits "owner/repo".
func ParseRepo(s string) (owner, repo string, err error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repository %q, want owner/repo", s)
	}
	return parts[0], parts[1], nil
}

// CurrentRepo infers owner/repo from the origin remote of the git repo in
// the working directory.
func CurrentRepo(run func(name string, args ...string) ([]byte, error)) (owner, repo string, err error) {
	out, err := run("git", "remote", "get-url", "origin")
	if err != nil {
		return "", "", fmt.Errorf("cannot infer repository from git remote (%w); pass owner/repo", err)
	}
	return InferRepo(string(out))
}
