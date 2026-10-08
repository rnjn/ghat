package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(defaultDeps(), os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the command line and returns the process exit code. An
// error is printed once to stderr, except an *exitError without a message:
// those (a failed conclusion, an ambiguous run already explained) are
// silent.
func run(d *deps, args []string, stdout, stderr io.Writer) int {
	cmd := newRootCmdWith(d)
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	err := cmd.Execute()
	var ee *exitError
	if err != nil && (!errors.As(err, &ee) || ee.err != nil) {
		_, _ = fmt.Fprintln(stderr, "ghat:", err)
	}
	return exitCode(err)
}
