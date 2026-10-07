package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootVersion(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "ghtui dev" {
		t.Fatalf("version output = %q, want %q", got, "ghtui dev")
	}
}
