package config

import (
	"os"
	"path/filepath"
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
