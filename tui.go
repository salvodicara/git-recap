package main

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
)

// errCancelled means the user picked "Cancel" in a hub menu — not an error,
// just "do nothing" for the caller to handle quietly.
var errCancelled = errors.New("cancelled")

// escKeyMap lets Esc exit a prompt exactly like Ctrl+C already does (both end
// up as huh.ErrUserAborted, handled identically wherever a form's error is
// checked). Not used for the two filterable repo multiselects (pickRepos,
// editProfile), which already bind Esc to "clear filter" while typing one.
var escKeyMap = func() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(key.WithKeys("esc", "ctrl+c"))
	return km
}()

// newForm is huh.NewForm with Esc wired up as an additional exit key.
func newForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).WithKeyMap(escKeyMap)
}

// hubRow is one selectable line in a settings/action menu. value, when set,
// renders the current value inline (e.g. "Recaps folder    ~/my-recaps") so
// the whole state is visible before picking what to change.
type hubRow struct {
	key   string
	label string
	value func() string
}

func (r hubRow) option() huh.Option[string] {
	label := r.label
	if r.value != nil {
		label = fmt.Sprintf("%-18s%s", r.label, r.value())
	}
	return huh.NewOption(label, r.key)
}

// runHub renders rows as a select menu and returns the chosen row's key.
func runHub(title string, rows []hubRow) (string, error) {
	opts := make([]huh.Option[string], len(rows))
	for i, r := range rows {
		opts[i] = r.option()
	}
	var choice string
	err := newForm(huh.NewGroup(
		huh.NewSelect[string]().Title(title).Options(opts...).Value(&choice),
	)).Run()
	return choice, err
}

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

// interactiveConfig is a settings hub: pick a setting, edit just it, land
// back on the hub. Nothing is scanned or saved until it's actually needed —
// repos are discovered lazily (once, cached, re-scanned only if the roots
// change) when Profiles is opened, and the file is written only on
// "Save & exit". "Quit without saving" (or Ctrl+C) leaves the on-disk config
// untouched.
func interactiveConfig(cfg *Config) error {
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	var cachedRepos []Repo
	var cachedFor string // roots cachedRepos was scanned for, joined

	reposFor := func() ([]Repo, error) {
		rootsKey := strings.Join(cfg.WorkspaceRoots, "\x00")
		if rootsKey == cachedFor {
			return cachedRepos, nil
		}
		fmt.Println("Scanning for git repos…")
		found := discoverRepos(cfg.WorkspaceRoots)
		if len(found) == 0 {
			return nil, fmt.Errorf("no git repos found under %s", strings.Join(cfg.WorkspaceRoots, ", "))
		}
		cachedRepos, cachedFor = found, rootsKey
		return found, nil
	}

	for {
		profSummary := "(none)"
		if names := profileNames(cfg); len(names) > 0 {
			profSummary = strings.Join(names, ", ")
		}
		defProf := cfg.DefaultProfile
		if defProf == "" {
			defProf = "(none)"
		}

		choice, err := runHub("Configure git-recap", []hubRow{
			{"roots", "Workspace roots", func() string { return strings.Join(cfg.WorkspaceRoots, ", ") }},
			{"recaps", "Recaps folder", cfg.recapsFolder},
			{"profiles", "Profiles", func() string { return profSummary }},
			{"default", "Default profile", func() string { return defProf }},
			{"save", "Save & exit", nil},
			{"quit", "Quit without saving", nil},
		})
		if err != nil {
			return err
		}

		switch choice {
		case "roots":
			if err := editWorkspaceRoots(cfg); err != nil {
				return err
			}
		case "recaps":
			if err := editRecapsFolder(cfg); err != nil {
				return err
			}
		case "profiles":
			repos, err := reposFor()
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			if err := manageProfiles(cfg, repos); err != nil {
				return err
			}
		case "default":
			if len(cfg.Profiles) == 0 {
				fmt.Println("No profiles yet — add one under Profiles first.")
				continue
			}
			if err := chooseDefault(cfg); err != nil {
				return err
			}
		case "save":
			if len(cfg.WorkspaceRoots) == 0 {
				fmt.Println("Error: at least one workspace root is required.")
				continue
			}
			if len(cfg.Profiles) == 0 {
				fmt.Println("Error: at least one profile is required.")
				continue
			}
			if cfg.DefaultProfile == "" {
				if err := chooseDefault(cfg); err != nil {
					return err
				}
			}
			path, err := saveConfig(cfg)
			if err != nil {
				return err
			}
			fmt.Printf("Wrote %s\nRun `git-recap` (or `git recap`) to generate your journal.\n", path)
			return nil
		case "quit":
			return nil
		}
	}
}

// editWorkspaceRoots edits cfg.WorkspaceRoots in place; rejects an empty list
// without discarding the previous value.
func editWorkspaceRoots(cfg *Config) error {
	val := strings.Join(cfg.WorkspaceRoots, ", ")
	if err := newForm(huh.NewGroup(
		huh.NewInput().
			Title("Workspace root(s) to scan").
			Description("Comma-separated; where your clones live.").
			Value(&val),
	)).Run(); err != nil {
		return err
	}
	if roots := splitCSV(val); len(roots) > 0 {
		cfg.WorkspaceRoots = roots
	} else {
		fmt.Println("Error: at least one workspace root is required — not changed.")
	}
	return nil
}

func editRecapsFolder(cfg *Config) error {
	val := cfg.recapsFolder()
	if err := newForm(huh.NewGroup(
		huh.NewInput().
			Title("Recaps folder").
			Description("Where recaps are written (its own git repo; commit it).").
			Value(&val),
	)).Run(); err != nil {
		return err
	}
	cfg.RecapsFolder = strings.TrimSpace(val)
	return nil
}

// manageProfiles loops a menu to add, edit, or delete profiles until "Done".
func manageProfiles(cfg *Config, repos []Repo) error {
	const newItem, doneItem = "+ New profile", "← Back"
	for {
		opts := append(profileNames(cfg), newItem, doneItem)
		var choice string
		if err := newForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("Profiles — edit one, add a new one, or go back").
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
	if err := newForm(huh.NewGroup(
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
		if err := newForm(huh.NewGroup(
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
	if err := newForm(huh.NewGroup(
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
	if err := newForm(huh.NewGroup(
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

// periodChoices is the shared preset menu — one source of truth for the
// generate hub's Period select and its display labels.
var periodChoices = []huh.Option[string]{
	huh.NewOption("Today", "today"),
	huh.NewOption("Yesterday", "yesterday"),
	huh.NewOption("This week", "week"),
	huh.NewOption("Last week", "last-week"),
	huh.NewOption("This month", "month"),
	huh.NewOption("Last month", "last-month"),
	huh.NewOption("This quarter", "quarter"),
	huh.NewOption("This year", "year"),
	huh.NewOption("Last 7 days", "last-7-days"),
	huh.NewOption("Last 30 days", "last-30-days"),
	huh.NewOption("Custom range…", "custom"),
}

func periodLabel(token string) string {
	for _, o := range periodChoices {
		if o.Value == token {
			return o.Key
		}
	}
	return token
}

// interactiveGenerate is the hub shown when git-recap runs bare on a
// terminal: pick a profile, a period, and (optionally) override the recaps
// folder for just this run — independently, in any order — then Generate,
// or Cancel to back out without writing anything. Repo fine-tuning stays on
// the explicit --pick flag. Chosen values are written back through the flag
// pointers; a Cancel returns errCancelled.
func interactiveGenerate(cfg *Config, profile, period, from, to, folder *string) error {
	names := profileNames(cfg)
	if len(names) == 0 {
		return fmt.Errorf("no profiles yet — run `git-recap config` to set one up")
	}

	sel := cfg.DefaultProfile
	if _, ok := cfg.Profiles[sel]; !ok {
		sel = names[0]
	}
	per := "month"
	var fromStr, toStr string
	folderOverride := *folder

	periodValue := func() string {
		if per == "custom" {
			if fromStr == "" || toStr == "" {
				return "Custom range… (not set)"
			}
			return fmt.Sprintf("%s → %s", fromStr, toStr)
		}
		return periodLabel(per)
	}
	folderValue := func() string {
		if folderOverride != "" {
			return folderOverride + "  (this run only)"
		}
		return cfg.recapsFolder()
	}

	for {
		rows := []hubRow{}
		if len(names) > 1 {
			rows = append(rows, hubRow{"profile", "Profile", func() string { return sel }})
		}
		rows = append(rows,
			hubRow{"period", "Period", periodValue},
			hubRow{"folder", "Recaps folder", folderValue},
			hubRow{"generate", "Generate", nil},
			hubRow{"cancel", "Cancel", nil},
		)

		choice, err := runHub("Build a recap", rows)
		if err != nil {
			return err
		}

		switch choice {
		case "profile":
			if err := newForm(huh.NewGroup(
				huh.NewSelect[string]().
					Title("Which profile?").
					Options(huh.NewOptions(names...)...).
					Value(&sel),
			)).Run(); err != nil {
				return err
			}
		case "period":
			if err := editPeriod(&per, &fromStr, &toStr); err != nil {
				return err
			}
		case "folder":
			if err := editRunRecapsFolder(&folderOverride, cfg.recapsFolder()); err != nil {
				return err
			}
		case "generate":
			if per == "custom" && (fromStr == "" || toStr == "") {
				fmt.Println("Error: set a custom From/To before generating.")
				continue
			}
			*profile, *folder = sel, folderOverride
			if per == "custom" {
				*from, *to = fromStr, toStr // resolveRange picks up the custom window
			} else {
				*period = per
			}
			return nil
		case "cancel":
			return errCancelled
		}
	}
}

// editRunRecapsFolder edits a per-run recaps-folder override. Prefilled with
// the configured default; leaving it unchanged (or blanking it back to that
// default) clears the override so the configured default is used.
func editRunRecapsFolder(override *string, configured string) error {
	val := *override
	if val == "" {
		val = configured
	}
	if err := newForm(huh.NewGroup(
		huh.NewInput().
			Title("Recaps folder for this run").
			Description("Overrides just this run — your saved default is unchanged.").
			Value(&val),
	)).Run(); err != nil {
		return err
	}
	val = strings.TrimSpace(val)
	if val == configured {
		val = "" // back to the configured default, no override
	}
	*override = val
	return nil
}

// editPeriod runs the period select, revealing a validated custom From/To
// pair only when "Custom range…" is chosen.
func editPeriod(per, fromStr, toStr *string) error {
	fromIn := huh.NewInput().
		Title("From (YYYY-MM-DD)").
		Placeholder("2026-01-31").
		Value(fromStr).
		Validate(validDate)
	toIn := huh.NewInput().
		Title("To (YYYY-MM-DD, inclusive)").
		Placeholder("2026-02-28").
		Value(toStr).
		Validate(func(s string) error {
			if err := validDate(s); err != nil {
				return err
			}
			f, _ := time.Parse("2006-01-02", *fromStr)
			t, _ := time.Parse("2006-01-02", s)
			if t.Before(f) {
				return fmt.Errorf("must be on or after From")
			}
			return nil
		})
	isCustom := func() bool { return *per != "custom" } // group hide-when-true

	return newForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Which period?").
				Description("How far back to recap.").
				Options(periodChoices...).
				Value(per),
		),
		huh.NewGroup(fromIn, toIn).WithHideFunc(isCustom),
	).Run()
}

// validDate rejects blank or non-YYYY-MM-DD input for the custom-range fields.
func validDate(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("required")
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return fmt.Errorf("use YYYY-MM-DD")
	}
	return nil
}
