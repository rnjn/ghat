package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

// testDeps returns deps with a fixed token, clock and git remote, and no
// real sleeping.
func testDeps() *deps {
	d := &deps{
		env: func(k string) string {
			if k == "GH_TOKEN" {
				return "tok"
			}
			return ""
		},
		run: func(name string, args ...string) ([]byte, error) {
			if name == "git" {
				return []byte("git@github.com:acme/api.git\n"), nil
			}
			return nil, errors.New("unexpected command " + name)
		},
		now:      func() time.Time { return testNow },
		cfgPath:  "/nonexistent/ghtui/config.yaml",
		cacheDir: "/nonexistent/ghtui-cache", // unreadable and unwritable
	}
	d.sleep = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	return d
}

// runCLI executes the root command against srv and returns its output.
func runCLI(t *testing.T, d *deps, srv *httptest.Server, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errb bytes.Buffer
	cmd := newRootCmdWith(d)
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	if srv != nil {
		args = append([]string{"--api-url", srv.URL}, args...)
	}
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), errb.String(), err
}

func serve(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}
