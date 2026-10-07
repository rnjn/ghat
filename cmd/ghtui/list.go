package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"ghtui/internal/gh"
)

func newListCmd(d *deps) *cobra.Command {
	var (
		opts   gh.RunsOpts
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "list [owner/repo]",
		Short: "List recent workflow runs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.PerPage < 1 || opts.PerPage > 100 {
				return fmt.Errorf("--limit must be between 1 and 100")
			}
			var arg string
			if len(args) == 1 {
				arg = args[0]
			}
			owner, repo, err := d.repo(arg)
			if err != nil {
				return err
			}
			c, err := d.client()
			if err != nil {
				return err
			}
			runs, _, err := c.ListRuns(cmd.Context(), owner, repo, opts, "")
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSONLines(cmd.OutOrStdout(), runs)
			}
			return writeRunsTable(cmd.OutOrStdout(), runs, d.now())
		},
	}
	cmd.Flags().StringVar(&opts.Branch, "branch", "", "only runs on this branch")
	cmd.Flags().StringVar(&opts.Status, "status", "", "only runs with this status or conclusion")
	cmd.Flags().IntVar(&opts.PerPage, "limit", 20, "maximum number of runs (1-100)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print one JSON object per run")
	return cmd
}
