package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Repos.PushedWithin.D() != 14*24*time.Hour {
		t.Errorf("PushedWithin = %v", c.Repos.PushedWithin)
	}
	if c.Poll.RunsActive.D() != 15*time.Second || c.Poll.RunsIdle.D() != time.Minute ||
		c.Poll.Jobs.D() != 5*time.Second || c.Poll.Logs.D() != 5*time.Second {
		t.Errorf("Poll = %+v", c.Poll)
	}
	if c.UI.ShowTimestamps {
		t.Error("ShowTimestamps default true")
	}
}

func TestLoadOverrides(t *testing.T) {
	p := writeFile(t, `
repos:
  pinned: [acme/api]
  exclude: [acme/old]
  pushed_within: 3d
poll:
  logs: 2s
ui:
  show_timestamps: true
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Repos.Pinned) != 1 || c.Repos.Pinned[0] != "acme/api" || c.Repos.Exclude[0] != "acme/old" {
		t.Errorf("Repos = %+v", c.Repos)
	}
	if c.Repos.PushedWithin.D() != 72*time.Hour {
		t.Errorf("PushedWithin = %v", c.Repos.PushedWithin.D())
	}
	if c.Poll.Logs.D() != 2*time.Second || c.Poll.Jobs.D() != 5*time.Second {
		t.Errorf("Poll = %+v (unset keys must keep defaults)", c.Poll)
	}
	if !c.UI.ShowTimestamps {
		t.Error("ShowTimestamps not set")
	}
}

func TestLoadBadYAML(t *testing.T) {
	if _, err := Load(writeFile(t, "repos: [unclosed")); err == nil {
		t.Fatal("want error for bad YAML")
	}
}

func TestLoadBadDuration(t *testing.T) {
	if _, err := Load(writeFile(t, "poll:\n  logs: soon\n")); err == nil {
		t.Fatal("want error for bad duration")
	}
}

func TestDefaultPathIsGhat(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	if got := DefaultPath(); got != "/tmp/xdg/ghat/config.yaml" {
		t.Fatalf("DefaultPath = %s", got)
	}
}

func TestLoadFallsBackToLegacyGhtuiConfig(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "ghtui", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("poll:\n  logs: 2s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(filepath.Join(dir, "ghat", "config.yaml"))
	if err != nil || c.Poll.Logs.D() != 2*time.Second {
		t.Fatalf("legacy config not used: %+v %v", c.Poll, err)
	}
}

func TestRunsPerRepo(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "none.yaml"))
	if err != nil || c.Poll.RunsPerRepo != 50 {
		t.Fatalf("default = %d (err %v), want 50", c.Poll.RunsPerRepo, err)
	}
	c, err = Load(writeFile(t, "poll:\n  runs_per_repo: 100\n"))
	if err != nil || c.Poll.RunsPerRepo != 100 {
		t.Fatalf("override = %d (err %v)", c.Poll.RunsPerRepo, err)
	}
	for _, bad := range []string{"0", "101", "-5"} {
		if _, err := Load(writeFile(t, "poll:\n  runs_per_repo: "+bad+"\n")); err == nil || !strings.Contains(err.Error(), "runs_per_repo") {
			t.Errorf("%s: err = %v, want a runs_per_repo error", bad, err)
		}
	}
}
