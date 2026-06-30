package main

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
)

// repoOptions builds a filterable multiselect option list, pre-checking repos
// whose path is in preChecked.
func repoOptions(repos []Repo, preChecked map[string]bool) []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(repos))
	for _, r := range repos {
		o := huh.NewOption(r.Slug(), r.Path)
		if preChecked[r.Path] {
			o = o.Selected(true)
		}
		opts = append(opts, o)
	}
	return opts
}

func reposByPath(repos []Repo, paths []string) []Repo {
	idx := make(map[string]Repo, len(repos))
	for _, r := range repos {
		idx[r.Path] = r
	}
	out := make([]Repo, 0, len(paths))
	for _, p := range paths {
		if r, ok := idx[p]; ok {
			out = append(out, r)
		}
	}
	return out
}

// pickRepos opens a fuzzy multiselect over discovered repos.
func pickRepos(repos []Repo, preChecked map[string]bool) ([]Repo, error) {
	var chosen []string
	err := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Select repos for this journal (type to filter, space to toggle)").
			Options(repoOptions(repos, preChecked)...).
			Filterable(true).
			Value(&chosen),
	)).Run()
	if err != nil {
		return nil, err
	}
	return reposByPath(repos, chosen), nil
}

// interactiveConfig is the on-rails editor for every setting: roots, journal
// root, profiles (add/edit/delete) and the default profile. It edits cfg in
// place (pre-filled from current values) and saves.
func interactiveConfig(cfg *Config) error {
	rootsStr := strings.Join(cfg.WorkspaceRoots, ", ")
	journal := cfg.JournalRoot
	if journal == "" {
		journal = "~/git-recap"
	}
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Workspace root(s) to scan").
			Description("Comma-separated; where your clones live.").
			Value(&rootsStr),
		huh.NewInput().
			Title("Journal root").
			Description("Where recaps are written.").
			Value(&journal),
	)).Run(); err != nil {
		return err
	}
	roots := splitCSV(rootsStr)
	if len(roots) == 0 {
		return fmt.Errorf("no workspace roots given")
	}
	cfg.WorkspaceRoots = roots
	cfg.JournalRoot = strings.TrimSpace(journal)

	fmt.Println("Scanning for git repos…")
	repos := discoverRepos(roots)
	if len(repos) == 0 {
		return fmt.Errorf("no git repos found under %s", strings.Join(roots, ", "))
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}

	if err := manageProfiles(cfg, repos); err != nil {
		return err
	}
	if len(cfg.Profiles) == 0 {
		return fmt.Errorf("no profiles configured")
	}
	if err := chooseDefault(cfg); err != nil {
		return err
	}

	path, err := saveConfig(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("Wrote %s\nRun `git-recap` (or `git recap`) to generate your journal.\n", path)
	return nil
}

// manageProfiles loops a menu to add, edit, or delete profiles until "Done".
func manageProfiles(cfg *Config, repos []Repo) error {
	const newItem, doneItem = "+ New profile", "✓ Done"
	for {
		opts := append(profileNames(cfg), newItem, doneItem)
		var choice string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("Profiles — edit one, add a new one, or finish").
				Options(huh.NewOptions(opts...)...).
				Value(&choice),
		)).Run(); err != nil {
			return err
		}
		switch choice {
		case doneItem:
			return nil
		case newItem:
			name, err := promptName()
			if err != nil {
				return err
			}
			p, err := editProfile(repos, Profile{})
			if err != nil {
				return err
			}
			cfg.Profiles[name] = p
		default:
			if err := editOrDelete(cfg, repos, choice); err != nil {
				return err
			}
		}
	}
}

func editOrDelete(cfg *Config, repos []Repo, name string) error {
	var action string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Profile: " + name).
			Options(huh.NewOptions("Edit", "Delete", "Cancel")...).
			Value(&action),
	)).Run(); err != nil {
		return err
	}
	switch action {
	case "Edit":
		p, err := editProfile(repos, cfg.Profiles[name])
		if err != nil {
			return err
		}
		cfg.Profiles[name] = p
	case "Delete":
		confirm := false
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().Title("Delete profile " + name + "?").Value(&confirm),
		)).Run(); err != nil {
			return err
		}
		if confirm {
			delete(cfg.Profiles, name)
			if cfg.DefaultProfile == name {
				cfg.DefaultProfile = ""
			}
		}
	}
	return nil
}

func promptName() (string, error) {
	var name string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("New profile name").Value(&name),
	)).Run(); err != nil {
		return "", err
	}
	if name = strings.TrimSpace(name); name == "" {
		return "", fmt.Errorf("profile name is required")
	}
	return name, nil
}

// editProfile picks repos (pre-checked from the existing profile) and emails,
// returning the new profile scope.
func editProfile(repos []Repo, existing Profile) (Profile, error) {
	preChecked := map[string]bool{}
	for _, r := range repos {
		if existing.includes(r) {
			preChecked[r.Path] = true
		}
	}
	emailsStr := strings.Join(existing.Emails, ", ")
	if emailsStr == "" {
		emailsStr = gitGlobalEmail()
	}

	var chosen []string
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Repos for this profile (type to filter, space to toggle)").
				Options(repoOptions(repos, preChecked)...).
				Filterable(true).
				Value(&chosen),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Author email(s)").
				Description("Comma-separated; only these authors' commits count.").
				Value(&emailsStr),
		),
	).Run(); err != nil {
		return Profile{}, err
	}
	if len(chosen) == 0 {
		return Profile{}, fmt.Errorf("no repos selected")
	}
	orgs, bareRepos := deriveScope(reposByPath(repos, chosen))
	return Profile{Orgs: orgs, Repos: bareRepos, Emails: splitCSV(emailsStr)}, nil
}

// chooseDefault sets default_profile: auto for a single profile, a picker
// otherwise (pre-selecting the current default).
func chooseDefault(cfg *Config) error {
	names := profileNames(cfg)
	if len(names) == 1 {
		cfg.DefaultProfile = names[0]
		return nil
	}
	def := cfg.DefaultProfile
	if def == "" {
		def = names[0]
	}
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Default profile").
			Options(huh.NewOptions(names...)...).
			Value(&def),
	)).Run(); err != nil {
		return err
	}
	cfg.DefaultProfile = def
	return nil
}

// deriveScope turns picked repos into a profile scope: unique orgs, plus the
// names of any org-less (local-only) repos so they still match.
func deriveScope(repos []Repo) (orgs, bareRepos []string) {
	orgSet, repoSet := map[string]bool{}, map[string]bool{}
	for _, r := range repos {
		if r.Org != "" {
			orgSet[r.Org] = true
		} else {
			repoSet[r.Name] = true
		}
	}
	return sortedSet(orgSet), sortedSet(repoSet)
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func gitGlobalEmail() string {
	out, _ := exec.Command("git", "config", "--global", "user.email").Output()
	return strings.TrimSpace(string(out))
}

// interactiveGenerate asks the few things a journal needs when git-recap is run
// bare on a terminal: which period, and which profile (only when there's more
// than one). Repo fine-tuning stays on the explicit --pick flag. Chosen values
// are written back through the flag pointers.
func interactiveGenerate(cfg *Config, profile, period *string) error {
	names := profileNames(cfg)
	if len(names) == 0 {
		return fmt.Errorf("no profiles yet — run `git-recap config` to set one up")
	}

	sel := cfg.DefaultProfile
	if _, ok := cfg.Profiles[sel]; !ok {
		sel = names[0]
	}
	per := "month"

	fields := []huh.Field{}
	if len(names) > 1 {
		fields = append(fields, huh.NewSelect[string]().
			Title("Which profile?").
			Options(huh.NewOptions(names...)...).
			Value(&sel))
	}
	fields = append(fields, huh.NewSelect[string]().
		Title("Which period?").
		Description("How far back to recap.").
		Options(huh.NewOptions("day", "week", "month", "quarter", "year")...).
		Value(&per))

	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		return err
	}
	*profile, *period = sel, per
	return nil
}
