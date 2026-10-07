package main

import (
	"context"
	"runtime"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"ghtui/internal/actions"
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
	msgs := make(chan any, 256)
	send := func(m any) {
		select {
		case msgs <- m:
		case <-ctx.Done():
		}
	}
	p := poller.New(c, st, cfg, send)
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
	if err != nil && ctx.Err() != nil && cmd.Context().Err() == nil {
		// The program ended because we cancelled after it quit; not an error.
		return nil
	}
	return err
}

// opener returns a function that opens a URL in the default browser.
func opener(goos string, run func(name string, args ...string) ([]byte, error)) func(string) error {
	name := "xdg-open"
	if goos == "darwin" {
		name = "open"
	}
	return func(url string) error {
		_, err := run(name, url)
		return err
	}
}
