package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the on-disk schema (config.toml). The init TUI writes it; users
// rarely hand-edit it.
type Config struct {
	WorkspaceRoots []string           `toml:"workspace_roots"`
	JournalRoot    string             `toml:"journal_root"`
	DefaultProfile string             `toml:"default_profile"`
	Profiles       map[string]Profile `toml:"profiles"`
}

// Profile bundles which repos to include (by GitHub org and/or repo name) and
// whose commits to count (by author email).
type Profile struct {
	Orgs   []string `toml:"orgs"`
	Repos  []string `toml:"repos"`
	Emails []string `toml:"emails"`
}

// configPath returns ~/.config/git-recap/config.toml (honouring XDG_CONFIG_HOME).
func configPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "git-recap", "config.toml"), nil
}

func loadConfig() (*Config, string, error) {
	path, err := configPath()
	if err != nil {
		return nil, "", err
	}
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return nil, path, err // includes os.IsNotExist — caller decides
	}
	return &c, path, nil
}

func saveConfig(c *Config) (string, error) {
	path, err := configPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(c); err != nil {
		return "", err
	}
	return path, nil
}

// expandTilde resolves a leading ~ to the user's home directory.
func expandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// includes reports whether a repo belongs to this profile (matched by org or by
// repo name, case-insensitive).
func (p Profile) includes(r Repo) bool {
	for _, o := range p.Orgs {
		if strings.EqualFold(o, r.Org) {
			return true
		}
	}
	for _, n := range p.Repos {
		if strings.EqualFold(n, r.Name) {
			return true
		}
	}
	return false
}

// profile looks up a profile by name, falling back to the default.
func (c *Config) profile(name string) (Profile, string, error) {
	if name == "" {
		name = c.DefaultProfile
	}
	p, ok := c.Profiles[name]
	if !ok {
		return Profile{}, name, fmt.Errorf("profile %q not found in config", name)
	}
	return p, name, nil
}
