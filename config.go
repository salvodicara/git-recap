package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// defaultConfig is the starting point for a brand-new config (no file yet).
func defaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		WorkspaceRoots: []string{filepath.Join(home, "Workspace")},
		JournalRoot:    "~/git-recap",
		Profiles:       map[string]Profile{},
	}
}

// isInteractive reports whether we can safely drive a terminal UI: both stdin
// and stdout must be a real terminal (not a pipe, file, CI, or agent).
func isInteractive() bool {
	return isTTY(os.Stdin) && isTTY(os.Stdout)
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// applyConfigFlags mutates cfg from the set of flags the user actually passed
// (set-semantics: a present flag replaces the field). Pure and testable.
func applyConfigFlags(cfg *Config, set map[string]string) error {
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	if v, ok := set["journal-root"]; ok {
		cfg.JournalRoot = strings.TrimSpace(v)
	}
	if v, ok := set["roots"]; ok {
		cfg.WorkspaceRoots = splitCSV(v)
	}
	if v, ok := set["default-profile"]; ok {
		cfg.DefaultProfile = strings.TrimSpace(v)
	}
	if name, ok := set["delete-profile"]; ok {
		if _, exists := cfg.Profiles[name]; !exists {
			return fmt.Errorf("profile %q not found", name)
		}
		delete(cfg.Profiles, name)
		if cfg.DefaultProfile == name {
			cfg.DefaultProfile = ""
		}
	}
	if name, ok := set["profile"]; ok {
		p := cfg.Profiles[name] // zero Profile if new
		if v, ok := set["orgs"]; ok {
			p.Orgs = splitCSV(v)
		}
		if v, ok := set["repos"]; ok {
			p.Repos = splitCSV(v)
		}
		if v, ok := set["emails"]; ok {
			p.Emails = splitCSV(v)
		}
		cfg.Profiles[name] = p
		if cfg.DefaultProfile == "" {
			cfg.DefaultProfile = name
		}
	} else {
		for _, k := range []string{"orgs", "repos", "emails"} {
			if _, ok := set[k]; ok {
				return fmt.Errorf("--%s requires --profile NAME", k)
			}
		}
	}
	if cfg.DefaultProfile != "" {
		if _, ok := cfg.Profiles[cfg.DefaultProfile]; !ok {
			return fmt.Errorf("default-profile %q is not a defined profile", cfg.DefaultProfile)
		}
	}
	return nil
}

const configUsage = `git-recap config — view or change configuration

With no flags on a terminal, opens an interactive editor (every setting).
With flags, applies them non-interactively (for scripts/agents). With no flags
and no terminal, prints the current config.

Flags (each replaces the field):
  --journal-root PATH      where recaps are written
  --roots A,B              workspace roots to scan (comma-separated)
  --default-profile NAME   profile used when none is given
  --profile NAME           create/update this profile, with:
    --orgs A,B               its orgs
    --repos X,Y              its repo names
    --emails a,b             its author emails
  --delete-profile NAME    remove a profile`

// runConfig is the single config command: interactive TUI, flag-driven edits,
// or a read-only dump when non-interactive with no flags.
func runConfig(argv []string) error {
	fs := flag.NewFlagSet("git-recap config", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, configUsage) }
	fs.String("journal-root", "", "set journal_root")
	fs.String("roots", "", "set workspace_roots (comma-separated)")
	fs.String("default-profile", "", "set default_profile")
	fs.String("profile", "", "profile to create/update")
	fs.String("orgs", "", "set profile orgs (comma-separated)")
	fs.String("repos", "", "set profile repos (comma-separated)")
	fs.String("emails", "", "set profile emails (comma-separated)")
	fs.String("delete-profile", "", "delete a profile")
	if err := fs.Parse(argv); err != nil {
		return err
	}

	cfg, path, err := loadConfig()
	if os.IsNotExist(err) {
		cfg, err = defaultConfig(), nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	set := map[string]string{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = f.Value.String() })

	if len(set) > 0 {
		if err := applyConfigFlags(cfg, set); err != nil {
			return err
		}
		p, err := saveConfig(cfg)
		if err != nil {
			return err
		}
		fmt.Printf("Updated %s\n", p)
		return nil
	}

	if !isInteractive() {
		fmt.Printf("# %s\n", path)
		return toml.NewEncoder(os.Stdout).Encode(cfg)
	}
	return interactiveConfig(cfg)
}

// profileNames returns the configured profile names, sorted.
func profileNames(cfg *Config) []string {
	out := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
