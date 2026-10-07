package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"ghtui/internal/gh"
	"ghtui/internal/tail"
)

func newTailCmd(d *deps) *cobra.Command {
	var (
		repoFlag   string
		noFollow   bool
		timestamps bool
		asJSON     bool
	)
	cmd := &cobra.Command{
		Use:   "tail <run-id|job-id>",
		Short: "Print a job's log as its steps complete",
		Long: "Print a job's log as its steps complete. GitHub only publishes a step's\n" +
			"output once the step finishes, so lines arrive a step at a time.\n" +
			"A run ID resolves to its only job or its only in-progress job.\n" +
			"Exit status: 0 on success, 1 on any other conclusion, 2 on error.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid ID %q", args[0])
			}
			cfg, err := d.config()
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("timestamps") {
				timestamps = cfg.UI.ShowTimestamps
			}
			owner, repo, err := d.repo(repoFlag)
			if err != nil {
				return err
			}
			c, err := d.client()
			if err != nil {
				return err
			}
			job, err := tail.ResolveJob(cmd.Context(), c, owner, repo, id)
			if errors.Is(err, tail.ErrAmbiguousRun) {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), err)
				return &exitError{code: 2}
			}
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			emit := func(l tail.LogLine) { _ = writeLogLine(out, l, timestamps, asJSON) }
			if noFollow {
				return printOnce(cmd, c, owner, repo, job, emit)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "tailing job %d %s (%s)\n", job.ID, job.Name, job.Status)
			t := tail.NewTailer(c, owner, repo, job.ID, tail.Options{Interval: cfg.Poll.Logs.D(), Sleep: d.sleep})
			final, err := t.Run(cmd.Context(), emit)
			if err != nil {
				return err
			}
			return conclusionExit(final.Conclusion)
		},
	}
	addRepoFlag(cmd, &repoFlag)
	cmd.Flags().BoolVar(&noFollow, "no-follow", false, "print the log so far and exit")
	cmd.Flags().BoolVar(&timestamps, "timestamps", false, "keep each line's timestamp")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print one JSON object per line")
	return cmd
}

// printOnce prints the log as it is now. A finished job exits with its
// conclusion; a running one exits 0.
func printOnce(cmd *cobra.Command, c *gh.Client, owner, repo string, job gh.Job, emit func(tail.LogLine)) error {
	raw, err := c.JobLog(cmd.Context(), owner, repo, job.ID)
	if err != nil && !errors.Is(err, gh.ErrLogNotReady) {
		return err
	}
	for _, l := range tail.ParseLines(raw, job.Steps) {
		emit(l)
	}
	if job.Status != "completed" {
		return nil
	}
	return conclusionExit(job.Conclusion)
}

// conclusionExit maps a GitHub conclusion to exit status 0 or 1.
func conclusionExit(conclusion string) error {
	if conclusion == "success" {
		return nil
	}
	return &exitError{code: 1}
}

var kindPrefix = map[tail.Kind]string{
	tail.Group:   "▸ ",
	tail.Error:   "error: ",
	tail.Warning: "warning: ",
	tail.Command: "$ ",
}

// writeLogLine renders one line as text or JSON. End-of-group markers carry
// no text and are dropped from text output.
func writeLogLine(w io.Writer, l tail.LogLine, timestamps, asJSON bool) error {
	if asJSON {
		return writeJSONLines(w, []tail.LogLine{l})
	}
	if l.Kind == tail.EndGroup {
		return nil
	}
	var ts string
	if timestamps && !l.Timestamp.IsZero() {
		ts = l.Timestamp.UTC().Format(time.RFC3339Nano) + " "
	}
	_, err := fmt.Fprintf(w, "%s%s%s\n", ts, kindPrefix[l.Kind], l.Text)
	return err
}
