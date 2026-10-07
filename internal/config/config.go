// Package config loads ~/.config/ghtui/config.yaml and infers repos from git.
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
	return c
}

// DefaultPath is $XDG_CONFIG_HOME/ghtui/config.yaml, else ~/.config/ghtui/config.yaml.
func DefaultPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "ghtui", "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ghtui", "config.yaml")
}

// Load reads path over the defaults. A missing file yields the defaults.
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}
