package tui

var helpLines = []string{
	"Keys",
	"",
	"  ↑/k ↓/j    move",
	"  enter      open",
	"  esc        back",
	"  w          watch run (bell when it finishes)",
	"  r          rerun run (failed jobs only if any failed)   asks y/N",
	"  x          cancel run                                   asks y/N",
	"  d          dispatch a workflow with inputs              asks y/N",
	"  o          open in browser",
	"  c          open the run's commit (diff) in browser",
	"  R          refresh this screen",
	"  tab ←/→    switch pane (jobs)",
	"  /          search the log · filter runs by branch or status",
	"  n N        next / previous search hit (log)",
	"  z Z        fold or unfold the group under the cursor / unfold all (log)",
	"  t          toggle timestamps (log)",
	"  G          follow log end (log)",
	"  ?          close help",
	"  q          quit",
}

func helpView(width, height int) string { return fit(helpLines, width, height) }
