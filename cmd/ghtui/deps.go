package main

import (
	"context"
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"ghtui/internal/cache"
	"ghtui/internal/config"
	"ghtui/internal/gh"
)

// deps are the process-level effects commands use, injected for tests.
type deps struct {
	env      func(string) string
	run      func(name string, args ...string) ([]byte, error)
	now      func() time.Time
	sleep    func(ctx context.Context, d time.Duration) error
	cfgPath  string
	cacheDir string
	apiURL   string
	tuiOpts  []tea.ProgramOption // test overrides for the TUI program
}

func defaultDeps() *deps {
	return &deps{
		env: os.Getenv,
		run: func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).Output()
		},
		now:      time.Now,
		cfgPath:  config.DefaultPath(),
		cacheDir: cache.Dir(),
	}
}

// client builds an authenticated GitHub client.
func (d *deps) client() (*gh.Client, error) {
	tok, err := gh.ResolveToken(d.env, d.run)
	if err != nil {
		return nil, err
	}
	var opts []gh.Option
	if d.apiURL != "" {
		opts = append(opts, gh.WithBaseURL(d.apiURL))
	}
	return gh.New(tok, opts...), nil
}

// repo resolves an "owner/repo" argument, inferring it from the working
// directory's git remote when empty.
func (d *deps) repo(arg string) (owner, repo string, err error) {
	if arg != "" {
		return config.ParseRepo(arg)
	}
	return config.CurrentRepo(d.run)
}

// config loads the user's config file.
func (d *deps) config() (config.Config, error) {
	return config.Load(d.cfgPath)
}

func addRepoFlag(cmd *cobra.Command, dst *string) {
	cmd.Flags().StringVar(dst, "repo", "", "repository as owner/repo (default: inferred from git remote)")
}
