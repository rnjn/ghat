package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"ghtui/internal/gh"
	"ghtui/internal/tail"
)

func newWatchCmd(d *deps) *cobra.Command {
	var (
		repoFlag string
		interval time.Duration
	)
	cmd := &cobra.Command{
		Use:   "watch <run-id>",
		Short: "Print run and job status changes until the run completes",
		Long: "Print one line per run or job status change until the run completes.\n" +
			"Exit status: 0 on success, 1 on any other conclusion, 2 on error.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid run ID %q", args[0])
			}
			if !cmd.Flags().Changed("interval") {
				cfg, err := d.config()
				if err != nil {
					return err
				}
				interval = cfg.Poll.Jobs.D()
			}
			if interval <= 0 {
				return fmt.Errorf("--interval must be positive")
			}
			owner, repo, err := d.repo(repoFlag)
			if err != nil {
				return err
			}
			c, err := d.client()
			if err != nil {
				return err
			}
			w := &watcher{c: c, owner: owner, repo: repo, runID: id, out: cmd.OutOrStdout(), now: d.now, jobs: map[int64]string{}}
			sleep := d.sleep
			if sleep == nil {
				sleep = tail.SleepContext
			}
			failures := 0
			for {
				run, err := w.poll(cmd.Context())
				switch {
				case err == nil:
					failures = 0
					if run.Status == "completed" {
						return conclusionExit(run.Conclusion)
					}
				case gh.IsTransient(err) && failures < maxTransient:
					failures++
				default:
					return err
				}
				if err := sleep(cmd.Context(), interval); err != nil {
					return err
				}
			}
		},
	}
	addRepoFlag(cmd, &repoFlag)
	cmd.Flags().DurationVar(&interval, "interval", 0, "poll interval (default: config poll.jobs, 5s)")
	return cmd
}

// maxTransient is how many consecutive 5xx or network errors a watch
// survives before giving up.
const maxTransient = 5

// watcher remembers the last printed state of a run and its jobs.
type watcher struct {
	c           *gh.Client
	owner, repo string
	runID       int64
	out         io.Writer
	now         func() time.Time
	run         string
	jobs        map[int64]string
}

// poll fetches the run and its jobs and prints what changed: jobs first,
// since a run completes only after its jobs do.
func (w *watcher) poll(ctx context.Context) (gh.Run, error) {
	run, err := w.c.GetRun(ctx, w.owner, w.repo, w.runID)
	if err != nil {
		return run, err
	}
	jobs, err := w.c.ListJobs(ctx, w.owner, w.repo, w.runID)
	if err != nil {
		return run, err
	}
	ts := w.now().Local().Format("15:04:05")
	for _, j := range jobs {
		if s := state(j.Status, j.Conclusion); w.jobs[j.ID] != s {
			w.jobs[j.ID] = s
			_, _ = fmt.Fprintf(w.out, "%s job %s %s\n", ts, j.Name, s)
		}
	}
	if s := state(run.Status, run.Conclusion); w.run != s {
		w.run = s
		_, _ = fmt.Fprintf(w.out, "%s run %s\n", ts, s)
	}
	return run, nil
}

func state(status, conclusion string) string {
	if status == "completed" && conclusion != "" {
		return status + " " + conclusion
	}
	return status
}
