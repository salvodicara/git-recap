// Command git-recap reconstructs a work journal from local git history.
// Invoked as `git-recap` or, because of the name, as `git recap`.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

const usage = `git-recap — reconstruct a work journal from git history

Usage:
  git-recap [flags]      generate a journal for a period
  git-recap init         first-run setup (scan repos, pick a profile)

Flags:
  --profile NAME         profile to use (default: config's default_profile)
  --org A,B              only these orgs (overrides profile selection)
  --repo X,Y             only these repo names (overrides profile selection)
  --period PERIOD        day|week|month|quarter|year (default: month)
  --from YYYY-MM-DD       custom range start (use with --to)
  --to YYYY-MM-DD         custom range end, inclusive (use with --from)
  --pick                 interactively fuzzy-pick repos for this run

With no flags, the default profile is used non-interactively.`

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		if err := runInit(); err != nil {
			fmt.Fprintln(os.Stderr, "init:", err)
			os.Exit(1)
		}
		return
	}
	if err := runGenerate(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "git-recap:", err)
		os.Exit(1)
	}
}

func runGenerate(argv []string) error {
	fs := flag.NewFlagSet("git-recap", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, usage) }
	var (
		profileFlag = fs.String("profile", "", "profile name")
		orgFlag     = fs.String("org", "", "comma-separated orgs")
		repoFlag    = fs.String("repo", "", "comma-separated repo names")
		period      = fs.String("period", "month", "day|week|month|quarter|year")
		fromFlag    = fs.String("from", "", "range start YYYY-MM-DD")
		toFlag      = fs.String("to", "", "range end YYYY-MM-DD (inclusive)")
		pick        = fs.Bool("pick", false, "interactively pick repos")
	)
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if !validPeriods[*period] {
		return fmt.Errorf("invalid --period %q (day|week|month|quarter|year)", *period)
	}

	cfg, cfgPath, err := loadConfig()
	if os.IsNotExist(err) {
		return fmt.Errorf("no config at %s — run `git-recap init` first", cfgPath)
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", cfgPath, err)
	}

	discovered := discoverRepos(cfg.WorkspaceRoots)
	if len(discovered) == 0 {
		return fmt.Errorf("no git repos found under %s", strings.Join(cfg.WorkspaceRoots, ", "))
	}

	orgs, repos := splitCSV(*orgFlag), splitCSV(*repoFlag)
	var (
		selected    []Repo
		profileName string
		emails      []string
	)

	if *pick {
		// Ad-hoc interactive selection; output filed under the default profile.
		profileName = cfg.DefaultProfile
		if profileName == "" {
			profileName = "adhoc"
		}
		preChecked := map[string]bool{}
		if p, ok := cfg.Profiles[profileName]; ok {
			emails = p.Emails
			for _, r := range discovered {
				if p.includes(r) {
					preChecked[r.Path] = true
				}
			}
		} else {
			emails = unionEmails(cfg)
		}
		if selected, err = pickRepos(discovered, preChecked); err != nil {
			return err
		}
	} else {
		// Non-interactive: a profile (named or default) scopes the repos;
		// --org/--repo narrow it further when given.
		prof, name, err := cfg.profile(*profileFlag)
		if err != nil {
			return err
		}
		profileName, emails = name, prof.Emails
		sel := Profile{Orgs: orgs, Repos: repos}
		if len(orgs) == 0 && len(repos) == 0 {
			sel = Profile{Orgs: prof.Orgs, Repos: prof.Repos}
		}
		for _, r := range discovered {
			if sel.includes(r) {
				selected = append(selected, r)
			}
		}
	}

	if len(selected) == 0 {
		return fmt.Errorf("no repos selected for profile %q", profileName)
	}

	from, to, year, name, err := resolveRange(*period, *fromFlag, *toFlag, time.Now())
	if err != nil {
		return err
	}

	var all []Commit
	for _, r := range selected {
		cs, err := scanCommits(r, from, to, emails)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: scanning %s: %v\n", r.Slug(), err)
			continue
		}
		all = append(all, cs...)
	}

	heading := fmt.Sprintf("%s — %s", profileName, name)
	out, err := writeJournal(cfg.JournalRoot, profileName, year, name, renderMarkdown(heading, all))
	if err != nil {
		return err
	}
	fmt.Printf("Wrote %s — %d commits across %d repos.\n", out, len(all), len(selected))
	return nil
}

// splitCSV splits "a, b ,c" into ["a","b","c"], dropping blanks.
func splitCSV(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// unionEmails collects every author email across all profiles (used for ad-hoc
// picker runs when no default profile is configured).
func unionEmails(cfg *Config) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range cfg.Profiles {
		for _, e := range p.Emails {
			if !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	return out
}
