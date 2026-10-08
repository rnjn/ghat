package main

import (
	"errors"

	"github.com/spf13/cobra"
)

var version = "dev"

// exitError carries a specific process exit code out of a command.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *exitError) Unwrap() error { return e.err }

// exitCode maps a command error to the process exit code: 0 for nil, the
// carried code for *exitError, otherwise 2 (usage or API error).
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return 2
}

func newRootCmd() *cobra.Command { return newRootCmdWith(defaultDeps()) }

func newRootCmdWith(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ghat",
		Short:         "Terminal UI and CLI for GitHub Actions",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}
	var here bool
	cmd.Flags().BoolVar(&here, "here", false, "open on the runs of the repo in the current directory")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error { return runTUI(cmd, d, here) }
	cmd.SetVersionTemplate("ghat {{.Version}}\n")
	cmd.PersistentFlags().StringVar(&d.apiURL, "api-url", "", "GitHub API base URL")
	_ = cmd.PersistentFlags().MarkHidden("api-url")
	cmd.AddCommand(newListCmd(d), newTailCmd(d), newWatchCmd(d))
	return cmd
}
