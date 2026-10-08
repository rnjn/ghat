package main

import (
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rnjn/ghat/internal/cache"
	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/store"
)

// slowAPI answers every request after a delay, so quitting happens while
// requests are in flight.
func slowAPI(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(300 * time.Millisecond):
		}
		if strings.HasPrefix(r.URL.Path, "/user/repos") {
			_, _ = w.Write([]byte(`[{"name":"api","owner":{"login":"acme"},"pushed_at":"` + time.Now().UTC().Format(time.RFC3339) + `"}]`))
			return
		}
		_, _ = w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
	}
}

func TestTUIQuitsPromptlyWithRequestsInFlight(t *testing.T) {
	base := runtime.NumGoroutine()
	d := testDeps()
	pr, pw := io.Pipe()
	d.tuiOpts = []tea.ProgramOption{tea.WithInput(pr), tea.WithOutput(io.Discard), tea.WithWindowSize(80, 20), tea.WithoutSignals()}
	srv := serve(t, slowAPI(t))

	done := make(chan error, 1)
	go func() {
		_, _, err := runCLI(t, d, srv)
		done <- err
	}()
	time.Sleep(100 * time.Millisecond) // discovery request is in flight
	_, _ = pw.Write([]byte("q"))
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ghat did not exit within 1s of q")
	}
	_ = pw.Close()
	srv.Close()
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > base+2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > base+2 {
		buf := make([]byte, 1<<16)
		t.Fatalf("goroutines: %d before, %d after\n%s", base, n, buf[:runtime.Stack(buf, true)])
	}
}

func TestTUIWithoutTokenExits2BeforeStarting(t *testing.T) {
	d := testDeps()
	d.env = func(string) string { return "" }
	d.run = func(string, ...string) ([]byte, error) { return nil, io.EOF }
	d.tuiOpts = []tea.ProgramOption{tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard)}
	_, _, err := runCLI(t, d, nil)
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "gh auth login") {
		t.Fatalf("exit %d err %v", exitCode(err), err)
	}
}

func TestOpenerPerOS(t *testing.T) {
	for goos, want := range map[string]string{
		"darwin": "open https://x", "linux": "xdg-open https://x", "freebsd": "xdg-open https://x",
		"windows": "rundll32 url.dll,FileProtocolHandler https://x",
	} {
		var got string
		open := opener(goos, func(name string, args ...string) ([]byte, error) {
			got = name + " " + strings.Join(args, " ")
			return nil, nil
		})
		if err := open("https://x"); err != nil || got != want {
			t.Errorf("%s: ran %q err %v, want %q", goos, got, err, want)
		}
	}
}

func TestTUIStartsFromCacheAndSavesOnQuit(t *testing.T) {
	dir := t.TempDir()
	st := store.New()
	st.SetRepos([]store.RepoState{{Repo: gh.Repo{Owner: "acme", Name: "api", PushedAt: time.Now()}}})
	st.SetRuns("acme/api", nil, `"cached-etag"`)
	savedAt := time.Now().Add(-time.Minute)
	if err := cache.Save(dir, cache.FromStore(st, savedAt)); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var discovered bool
	var etag string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasPrefix(r.URL.Path, "/user/repos"):
			discovered = true
			_, _ = fmt.Fprintf(w, `[{"name":"api","owner":{"login":"acme"},"pushed_at":%q}]`, time.Now().UTC().Format(time.RFC3339))
		case r.URL.Path == "/repos/acme/api/actions/runs":
			etag = r.Header.Get("If-None-Match")
			w.WriteHeader(http.StatusNotModified)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	d := testDeps()
	d.cacheDir = dir
	pr, pw := io.Pipe()
	d.tuiOpts = []tea.ProgramOption{tea.WithInput(pr), tea.WithOutput(io.Discard), tea.WithWindowSize(80, 20), tea.WithoutSignals()}
	done := make(chan error, 1)
	go func() { _, _, err := runCLI(t, d, srv); done <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := etag
		mu.Unlock()
		if got != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, _ = pw.Write([]byte("q"))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !discovered || etag != `"cached-etag"` {
		t.Fatalf("discovered=%v etag=%q: startup must always rediscover (config, --here or account may have changed) and reuse cached ETags", discovered, etag)
	}
	snap, ok := cache.Load(dir)
	if !ok || !snap.SavedAt.After(savedAt) {
		t.Fatalf("cache not saved on quit (ok=%v savedAt=%v)", ok, snap.SavedAt)
	}
}

func TestHereRejectsNonGitHubRemoteBeforeTUI(t *testing.T) {
	for name, run := range map[string]func(string, ...string) ([]byte, error){
		"no git": func(string, ...string) ([]byte, error) { return nil, io.EOF },
		"gitlab": func(string, ...string) ([]byte, error) { return []byte("git@gitlab.com:o/r.git\n"), nil },
	} {
		d := testDeps()
		d.run = run
		d.tuiOpts = []tea.ProgramOption{tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard)}
		_, _, err := runCLI(t, d, nil, "--here")
		if exitCode(err) != 2 || strings.Contains(err.Error(), "\n") {
			t.Fatalf("%s: exit %d err %v", name, exitCode(err), err)
		}
	}
}

func TestHereEnsuresRepoIsPolled(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path] = true
		mu.Unlock()
		switch r.URL.Path {
		case "/user/repos":
			_, _ = w.Write([]byte(`[]`)) // not recently pushed
		case "/repos/acme/api":
			_, _ = w.Write([]byte(`{"name":"api","owner":{"login":"acme"}}`))
		default:
			_, _ = w.Write([]byte(`{"total_count":0,"workflow_runs":[]}`))
		}
	})
	d := testDeps()
	pr, pw := io.Pipe()
	d.tuiOpts = []tea.ProgramOption{tea.WithInput(pr), tea.WithOutput(io.Discard), tea.WithWindowSize(80, 20), tea.WithoutSignals()}
	done := make(chan error, 1)
	go func() { _, _, err := runCLI(t, d, srv, "--here"); done <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := seen["/repos/acme/api/actions/runs"]
		mu.Unlock()
		if ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	go func() { _, _ = pw.Write([]byte("q")) }() // never block if the program already exited
	err := <-done
	_ = pr.Close()
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !seen["/repos/acme/api/actions/runs"] {
		t.Fatalf("--here repo never polled: %v", seen)
	}
}
