package main

import (
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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
		t.Fatal("ghtui did not exit within 1s of q")
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
