package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// runInit is the first-run setup: ask for roots, scan, pick repos, name the
// profile, set author emails, and write config.toml.
func runInit() error {
	home, _ := os.UserHomeDir()
	rootsStr := filepath.Join(home, "Workspace")
	emailsStr := gitGlobalEmail()

	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Workspace root(s) to scan").
			Description("Comma-separated; where your clones live.").
			Value(&rootsStr),
	)).Run(); err != nil {
		return err
	}
	roots := splitCSV(rootsStr)
	if len(roots) == 0 {
		return fmt.Errorf("no workspace roots given")
	}

	fmt.Println("Scanning for git repos…")
	repos := discoverRepos(roots)
	if len(repos) == 0 {
		return fmt.Errorf("no git repos found under %s", strings.Join(roots, ", "))
	}

	var chosen []string
	profileName := "work"
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(fmt.Sprintf("Found %d repos — pick the ones for this profile", len(repos))).
				Options(repoOptions(repos, nil)...).
				Filterable(true).
				Value(&chosen),
		),
		huh.NewGroup(
			huh.NewInput().Title("Profile name").Value(&profileName),
			huh.NewInput().
				Title("Your author email(s)").
				Description("Comma-separated; only commits by these authors are counted.").
				Value(&emailsStr),
		),
	).Run(); err != nil {
		return err
	}

	if len(chosen) == 0 {
		return fmt.Errorf("no repos selected")
	}
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return fmt.Errorf("profile name is required")
	}

	orgs, bareRepos := deriveScope(reposByPath(repos, chosen))
	cfg := &Config{
		WorkspaceRoots: roots,
		JournalRoot:    "~/git-recap",
		DefaultProfile: profileName,
		Profiles: map[string]Profile{
			profileName: {Orgs: orgs, Repos: bareRepos, Emails: splitCSV(emailsStr)},
		},
	}
	path, err := saveConfig(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("Wrote %s\nRun `git-recap` (or `git recap`) to generate this month's journal.\n", path)
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
