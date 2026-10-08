package main

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// pseudoHash matches the commit hash at the end of a Go pseudo-version,
// e.g. v0.1.2-0.20261008034317-c1a4a448eeca.
var pseudoHash = regexp.MustCompile(`\d{14}-([0-9a-f]{12})$`)

// buildVersion describes this binary from the build info Go records.
func buildVersion() string {
	bi, _ := debug.ReadBuildInfo()
	return describeBuild(bi)
}

// describeBuild renders a version and short commit: "v0.1.1" for a tagged
// module build (Go records no commit for those), "<pseudo> · c1a4a44" for
// an untagged one, and "dev · c1a4a44" (with * when dirty) for a checkout.
func describeBuild(bi *debug.BuildInfo) string {
	if bi == nil {
		return "dev"
	}
	// Local builds since Go 1.24 carry "+dirty"; the commit's * says it.
	version := strings.TrimSuffix(bi.Main.Version, "+dirty")
	if version == "" || version == "(devel)" {
		version = "dev"
	}
	var commit string
	dirty := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if commit == "" {
		if m := pseudoHash.FindStringSubmatch(version); m != nil {
			commit = m[1]
		}
	}
	if commit == "" {
		return version
	}
	if len(commit) > 7 {
		commit = commit[:7]
	}
	if dirty {
		commit += "*"
	}
	return version + " · " + commit
}
