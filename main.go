// Command git-recap reconstructs a work journal from local git history.
// Invoked as `git-recap` or, because of the name, as `git recap`.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"
)

// version is stamped by release builds via -ldflags "-X main.version=vX.Y.Z"
// (goreleaser and the Homebrew formula). Left empty for `go install`, where it
// falls back to the module/VCS info the Go toolchain embeds automatically.
var version = ""

// versionString reports the build version across every install channel:
// the ldflags stamp if present, else the installed module version, else the
// VCS revision of a local build.
func versionString() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(unknown)"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v // installed via `go install module@version`
	}
	var rev, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		return "devel-" + rev + dirty
	}
	return "(devel)"
}

const usage = `git-recap — reconstruct a work journal from git history

Usage:
  git-recap [flags]      generate a journal (no flags on a terminal = interactive)
  git-recap config       view or change configuration
  git-recap version      print the version (also --version, -v)

Generate flags:
  --profile NAME         profile to use (default: config's default_profile)
  --org A,B              only these orgs (overrides profile selection)
  --repo X,Y             only these repo names (overrides profile selection)
  --period PERIOD        a period preset (default: month). One of:
                           day/today, yesterday,
                           week/this-week, last-week,
                           month/this-month, last-month,
                           quarter, year,
                           last-7-days, last-30-days
  --from YYYY-MM-DD       custom range start (use with --to)
  --to YYYY-MM-DD         custom range end, inclusive (use with --from)
  --pick                 interactively fuzzy-pick repos for this run

Configure — edit single fields from the CLI (git-recap config ...):
  (no flags on a terminal opens an interactive editor for every setting)
  --journal-root PATH    where recaps are written
  --roots A,B            workspace roots to scan
  --default-profile NAME profile used when none is given
  --profile NAME         create/update a profile (with --orgs/--repos/--emails)
  --delete-profile NAME  remove a profile

Run bare on a terminal to pick profile and period interactively; piped or with
any flag, git-recap runs non-interactively.`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "config":
			if err := runConfig(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "config:", err)
				os.Exit(1)
			}
			return
		case "version", "--version", "-v":
			fmt.Printf("git-recap %s\n", versionString())
			return
		}
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

	cfg, cfgPath, err := loadConfig()
	if os.IsNotExist(err) {
		return fmt.Errorf("no config at %s — run `git-recap config` first", cfgPath)
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", cfgPath, err)
	}

	// Bare run on a terminal → interactive selection. Any flag, or no TTY
	// (pipe/cron/agent), keeps the non-interactive path below.
	interactive := fs.NFlag() == 0 && isInteractive()
	if interactive {
		if err := interactiveGenerate(cfg, profileFlag, period, fromFlag, toFlag); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Scanning your workspace for repos…")
	}

	// --from/--to (a custom window) take priority over --period, so only
	// validate the period token when no explicit range is given.
	if *fromFlag == "" && !validPeriod(*period) {
		return fmt.Errorf("invalid --period %q (day|week|month|quarter|year, today, yesterday, this-week, last-week, this-month, last-month, last-7-days, last-30-days)", *period)
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
		// Ad-hoc interactive selection; filed under the chosen/default profile.
		profileName = *profileFlag
		if profileName == "" {
			profileName = cfg.DefaultProfile
		}
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
