package tui

var helpLines = []string{
	"Keys",
	"",
	"  ↑/k ↓/j    move   ←/h →/l columns (graph)",
	"  enter      open",
	"  esc        back",
	"  w          watch run (bell when it finishes)",
	"  r          rerun run (failed jobs only if any failed)   asks y/N",
	"  x          cancel run                                   asks y/N",
	"  d          dispatch a workflow with inputs              asks y/N",
	"  o  O       copy the link / open in browser",
	"  c  C       copy the run's commit (diff) link / open it",
	"  v          view the run as a graph or timeline (runs, pipeline)",
	"  tab        switch graph / timeline (view) · switch pane (jobs)",
	"  R          refresh this screen",
	"  /          search the log · filter runs: terms match branch or status,",
	"             -term hides, aliases failed / active / done (e.g. /main -skipped)",
	"  s          cycle a status preset (runs): failed, active, queued, done, all",
	"  n N        next / previous search hit (log)",
	"  z Z        fold or unfold the group under the cursor / unfold all (log)",
	"  t          toggle timestamps (log)",
	"  G          follow log end (log)",
	"  ?          close help",
	"  q          quit",
}

func helpView(width, height int) string { return fit(helpLines, width, height) }
