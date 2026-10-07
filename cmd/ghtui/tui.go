package main

import (
	"context"
	"runtime"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"ghtui/internal/actions"
	"ghtui/internal/cache"
	"ghtui/internal/poller"
	"ghtui/internal/store"
	"ghtui/internal/tui"
)

// runTUI starts the poller and the Bubble Tea program, and stops both when
// the program exits.
func runTUI(cmd *cobra.Command, d *deps) error {
	c, err := d.client()
	if err != nil {
		return err
	}
	cfg, err := d.config()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	st := store.New()
	var popts []poller.Option
	if snap, ok := cache.Load(d.cacheDir); ok {
		snap.Apply(st)
		popts = append(popts, poller.WithLastDiscovery(snap.SavedAt))
	}
	// Cache write failures are ignored: the cache is only an optimisation.
	saveCache := func() { _ = cache.Save(d.cacheDir, cache.FromStore(st, time.Now())) }
	popts = append(popts, poller.WithAfterDiscovery(saveCache))
	msgs := make(chan any, 256)
	send := func(m any) {
		select {
		case msgs <- m:
		case <-ctx.Done():
		}
	}
	p := poller.New(c, st, cfg, send, popts...)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.Run(ctx)
	}()

	model := tui.NewModel(tui.Context{
		Store: st, Refresh: p.Refresh, Now: time.Now, ShowTimestamps: cfg.UI.ShowTimestamps,
		Actions: actions.New(c), Open: opener(runtime.GOOS, d.run),
	}, msgs)
	opts := append([]tea.ProgramOption{tea.WithContext(ctx)}, d.tuiOpts...)
	_, err = tea.NewProgram(model, opts...).Run()
	cancel()
	wg.Wait()
	close(msgs) // poller is stopped; this releases the TUI's channel reader
	saveCache()
	if err != nil && ctx.Err() != nil && cmd.Context().Err() == nil {
		// The program ended because we cancelled after it quit; not an error.
		return nil
	}
	return err
}

// opener returns a function that opens a URL in the default browser.
func opener(goos string, run func(name string, args ...string) ([]byte, error)) func(string) error {
	return func(url string) error {
		var err error
		switch goos {
		case "darwin":
			_, err = run("open", url)
		case "windows":
			_, err = run("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			_, err = run("xdg-open", url)
		}
		return err
	}
}
