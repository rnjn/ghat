package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	err := newRootCmd().Execute()
	var ee *exitError
	if err != nil && (!errors.As(err, &ee) || ee.err != nil) {
		fmt.Fprintln(os.Stderr, "ghat:", err)
	}
	os.Exit(exitCode(err))
}
