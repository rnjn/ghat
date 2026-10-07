package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/rnjn/ghtui/internal/gh"
)

// runState is the conclusion of a completed run, else its status.
func runState(r gh.Run) string {
	if r.Status == "completed" && r.Conclusion != "" {
		return r.Conclusion
	}
	return r.Status
}

// runDuration is wall time from creation to last update for completed runs,
// and to now for active ones.
func runDuration(r gh.Run, now time.Time) time.Duration {
	end := now
	if r.Status == "completed" {
		end = r.UpdatedAt
	}
	if end.Before(r.CreatedAt) {
		return 0
	}
	return end.Sub(r.CreatedAt).Truncate(time.Second)
}

func writeRunsTable(w io.Writer, runs []gh.Run, now time.Time) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "STATUS\tWORKFLOW\tBRANCH\tEVENT\tACTOR\tDURATION\tRUN\tID")
	for _, r := range runs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t#%d\t%d\n",
			runState(r), r.WorkflowName, r.Branch, r.Event, r.Actor, runDuration(r, now), r.Number, r.ID)
	}
	return tw.Flush()
}

// writeJSONLines writes one JSON object per line.
func writeJSONLines[T any](w io.Writer, items []T) error {
	enc := json.NewEncoder(w)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			return err
		}
	}
	return nil
}
