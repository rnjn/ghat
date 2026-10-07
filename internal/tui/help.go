package tui

var helpLines = []string{
	"Keys",
	"",
	"  ↑/k ↓/j    move",
	"  enter      open",
	"  esc        back",
	"  w          watch run (bell when it finishes)",
	"  R          refresh this screen",
	"  tab ←/→    switch pane (jobs)",
	"  t          toggle timestamps (log)",
	"  G          follow log end (log)",
	"  ?          close help",
	"  q          quit",
}

func helpView(width, height int) string { return fit(helpLines, width, height) }
