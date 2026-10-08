package main

import (
	"runtime/debug"
	"testing"
)

func info(version string, settings ...string) *debug.BuildInfo {
	bi := &debug.BuildInfo{Main: debug.Module{Path: "github.com/rnjn/ghat", Version: version}}
	for i := 0; i+1 < len(settings); i += 2 {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: settings[i], Value: settings[i+1]})
	}
	return bi
}

func TestDescribeBuild(t *testing.T) {
	for name, tc := range map[string]struct {
		bi   *debug.BuildInfo
		want string
	}{
		"tagged release":      {info("v0.1.1"), "v0.1.1"},
		"pseudo-version":      {info("v0.1.2-0.20261008034317-c1a4a448eeca"), "v0.1.2-0.20261008034317-c1a4a448eeca · c1a4a44"},
		"local clean":         {info("(devel)", "vcs.revision", "c1a4a448eecac24f35da8eaecaea698ea8cce80f", "vcs.modified", "false"), "dev · c1a4a44"},
		"local dirty":         {info("(devel)", "vcs.revision", "c1a4a448eecac24f35da8eaecaea698ea8cce80f", "vcs.modified", "true"), "dev · c1a4a44*"},
		"no vcs info":         {info("(devel)"), "dev"},
		"no build info":       {nil, "dev"},
		"tag plus vcs":        {info("v0.2.0", "vcs.revision", "abcdef0123456789"), "v0.2.0 · abcdef0"},
		"local on tag, dirty": {info("v0.1.1+dirty", "vcs.revision", "c1a4a448eecac24f", "vcs.modified", "true"), "v0.1.1 · c1a4a44*"},
		"local after tag":     {info("v0.1.2-0.20261008034317-c1a4a448eeca+dirty", "vcs.revision", "c1a4a448eecac24f", "vcs.modified", "true"), "v0.1.2-0.20261008034317-c1a4a448eeca · c1a4a44*"},
	} {
		if got := describeBuild(tc.bi); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}
