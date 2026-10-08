package poller

import (
	"context"
	"testing"
	"time"

	"github.com/rnjn/ghat/internal/config"
	"github.com/rnjn/ghat/internal/store"
)

// waitCalls waits until the fake has seen n calls starting with prefix.
func waitCalls(t *testing.T, api *fakeAPI, prefix string, n int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		api.mu.Lock()
		got := countCalls(api.calls, prefix)
		api.mu.Unlock()
		if got >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	t.Fatalf("wanted %d %q calls within %v, got %v", n, prefix, within, api.calls)
}

func TestRunPollsWakesOnRefreshAndStopsOnCancel(t *testing.T) {
	api := newFake()
	api.repos = append(api.repos, ghRepo("a", "x", time.Hour))
	p := New(api, store.New(), config.Default(), func(any) {})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()

	waitCalls(t, api, "ListRepos", 1, time.Second)
	// The next discovery is 10 minutes away; Refresh must wake the loop at
	// once, well before the 1 s ticker.
	p.Refresh("repos")
	waitCalls(t, api, "ListRepos", 2, 300*time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRefreshNeverBlocks(t *testing.T) {
	p := New(newFake(), store.New(), config.Default(), func(any) {})
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ { // nobody is running the loop to drain wake
			p.Refresh("repos")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Refresh blocked")
	}
}
