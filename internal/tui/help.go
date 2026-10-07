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
	"  R          refresh this screen",
	"  tab ←/→    switch pane (jobs)",
	"  t          toggle timestamps (log)",
	"  G          follow log end (log)",
	"  ?          close help",
	"  q          quit",
}

func helpView(width, height int) string { return fit(helpLines, width, height) }
