// Package config loads ~/.config/ghat/config.yaml and infers repos from git.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration that also accepts whole days, e.g. "14d".
type Duration time.Duration

// D returns the value as a time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := parseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	*d = Duration(v)
	return nil
}

func parseDuration(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return v, nil
}

// Config mirrors the YAML file; every key is optional.
type Config struct {
	Repos struct {
		Pinned       []string `yaml:"pinned"`
		Exclude      []string `yaml:"exclude"`
		PushedWithin Duration `yaml:"pushed_within"`
	} `yaml:"repos"`
	Poll struct {
		RunsActive Duration `yaml:"runs_active"`
		RunsIdle   Duration `yaml:"runs_idle"`
		Jobs       Duration `yaml:"jobs"`
		Logs       Duration `yaml:"logs"`
		// RunsPerRepo is how many recent runs the TUI shows per repo
		// (1–100, GitHub's page size limit).
		RunsPerRepo int `yaml:"runs_per_repo"`
	} `yaml:"poll"`
	UI struct {
		ShowTimestamps bool `yaml:"show_timestamps"`
	} `yaml:"ui"`
}

// Default returns the built-in configuration.
func Default() Config {
	var c Config
	c.Repos.PushedWithin = Duration(14 * 24 * time.Hour)
	c.Poll.RunsActive = Duration(15 * time.Second)
	c.Poll.RunsIdle = Duration(60 * time.Second)
	c.Poll.Jobs = Duration(5 * time.Second)
	c.Poll.Logs = Duration(5 * time.Second)
	c.Poll.RunsPerRepo = 50
	return c
}

// DefaultPath is $XDG_CONFIG_HOME/ghat/config.yaml, else ~/.config/ghat/config.yaml.
func DefaultPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "ghat", "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ghat", "config.yaml")
}

// legacyName is the tool's former name; its config is still read when the
// new one does not exist.
const legacyName = "ghtui"

// Load reads path over the defaults. A missing file yields the defaults,
// unless a config from before the rename sits next to it.
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		dir := filepath.Dir(path)
		if filepath.Base(dir) == "ghat" {
			b, err = os.ReadFile(filepath.Join(filepath.Dir(dir), legacyName, filepath.Base(path)))
		}
	}
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if n := c.Poll.RunsPerRepo; n < 1 || n > 100 {
		return c, fmt.Errorf("%s: poll.runs_per_repo must be between 1 and 100, got %d", path, n)
	}
	return c, nil
}
