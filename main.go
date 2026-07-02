// Command git-recap reconstructs a work journal from local git history.
// Invoked as `git-recap` or, because of the name, as `git recap`.
package main

import (
	"errors"
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

const usage = `git-recap — instant recap of your git work, and a journal that writes itself

Usage:
  git-recap              standup recap: everything since your last working day
  git-recap [flags]      recap any period (prints to stdout; --write also saves)
  git-recap -i           interactive builder: pick profile/period, save a file
  git-recap index        rebuild index.html + per-period pages in the recaps folder
  git-recap config       view or change configuration
  git-recap version      print the version (also --version, -v)

Flags:
  --period PERIOD        standup (default), day/today, yesterday,
                         week/this-week, last-week, month/this-month,
                         last-month, quarter, year, last-7-days, last-30-days
  --from / --to          custom range instead, YYYY-MM-DD (--to inclusive)
  --profile NAME         profile to use (default: config's default_profile)
  --org A,B              only these orgs (overrides profile selection)
  --repo X,Y             only these repo names (overrides profile selection)
  --pick                 interactively fuzzy-pick repos for this run
  --fetch                git fetch each repo first (work pushed elsewhere shows up)
  --diffstat             include files changed and +/− lines per commit
  --format F             stdout format: term (default on a terminal), md, json,
                         or html (self-contained report with a heatmap)
  --write                also save the recap as markdown in your recaps folder
  --recaps-folder PATH   save there instead of the configured folder
                         (implies --write; one-off, not saved)

Zero config: without a config file, git-recap scans the current directory and
counts commits by your git user.email. Run ` + "`git-recap config`" + ` to set up
workspace roots, profiles, and a recaps folder worth keeping in git.`

func main() {
	if len(os.Args) > 1 {
		switch arg := os.Args[1]; arg {
		case "config":
			if err := runConfig(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "config:", err)
				os.Exit(1)
			}
			return
		case "index":
			if err := runIndex(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "index:", err)
				os.Exit(1)
			}
			return
		case "version", "--version", "-v":
			fmt.Printf("git-recap %s\n", versionString())
			return
		case "help", "--help", "-h":
			fmt.Println(usage)
			return
		default:
			if !strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "git-recap: unknown command %q\n\n%s\n", arg, usage)
				os.Exit(1)
			}
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
		period      = fs.String("period", "standup", "period preset")
		fromFlag    = fs.String("from", "", "range start YYYY-MM-DD")
		toFlag      = fs.String("to", "", "range end YYYY-MM-DD (inclusive)")
		pick        = fs.Bool("pick", false, "interactively pick repos")
		fetch       = fs.Bool("fetch", false, "git fetch each repo before scanning")
		diffstat    = fs.Bool("diffstat", false, "include files changed and +/− lines per commit")
		write       = fs.Bool("write", false, "also save the recap to the recaps folder")
		format      = fs.String("format", "", "stdout format: term|md|json")
		folderFlag  = fs.String("recaps-folder", "", "recaps folder for this run (implies --write)")
	)
	var interactive bool
	fs.BoolVar(&interactive, "i", false, "interactive recap builder")
	fs.BoolVar(&interactive, "interactive", false, "interactive recap builder")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (see `git-recap help`)", fs.Arg(0))
	}
	if *folderFlag != "" {
		*write = true
	}

	// Fail fast on a bad format, before any scanning.
	outFormat := *format
	if outFormat == "" {
		outFormat = "md"
		if isTTY(os.Stdout) {
			outFormat = "term"
		}
	}
	switch outFormat {
	case "term", "md", "markdown", "json", "html":
	default:
		return fmt.Errorf("invalid --format %q (term|md|json|html)", outFormat)
	}

	cfg, cfgPath, err := loadConfig()
	zeroConfig := os.IsNotExist(err)
	if err != nil && !zeroConfig {
		return fmt.Errorf("reading %s: %w", cfgPath, err)
	}
	if zeroConfig {
		// No config: recap the current directory with your git identity.
		// Anything that needs profiles or a saved recaps folder needs config.
		if interactive || *profileFlag != "" || (*write && *folderFlag == "") {
			return fmt.Errorf("no config at %s — run `git-recap config` first", cfgPath)
		}
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		// From inside a repo's subdirectory, recap the whole repo — like git.
		if top := repoToplevel(wd); top != "" {
			wd = top
		}
		cfg = &Config{WorkspaceRoots: []string{wd}}
	}

	if interactive {
		if !isInteractive() {
			return fmt.Errorf("-i needs a terminal")
		}
		if err := interactiveGenerate(cfg, profileFlag, period, fromFlag, toFlag, folderFlag); err != nil {
			if errors.Is(err, errCancelled) {
				fmt.Println("Cancelled.")
				return nil
			}
			return err
		}
		*write = true // the builder exists to produce a journal file
		fmt.Fprintln(os.Stderr, "Scanning your workspace for repos…")
	}

	// --from/--to (a custom window) take priority over --period, so only
	// validate the period token when no explicit range is given.
	if *fromFlag == "" && !validPeriod(*period) {
		return fmt.Errorf("invalid --period %q (standup, day|today, yesterday, week|this-week, last-week, month|this-month, last-month, quarter, year, last-7-days, last-30-days)", *period)
	}

	discovered := discoverRepos(cfg.WorkspaceRoots)
	if len(discovered) == 0 {
		hint := ""
		if zeroConfig {
			hint = " (scanned the current directory; run `git-recap config` to set workspace roots)"
		}
		return fmt.Errorf("no git repos found under %s%s", strings.Join(cfg.WorkspaceRoots, ", "), hint)
	}

	orgs, repos := splitCSV(*orgFlag), splitCSV(*repoFlag)
	var (
		selected    []Repo
		profileName string
		emails      []string
	)

	switch {
	case zeroConfig:
		profileName = "recap"
		if e := gitEmail(cfg.WorkspaceRoots[0]); e != "" {
			emails = []string{e}
		} else {
			fmt.Fprintln(os.Stderr, "warning: git user.email is unset — counting all authors")
		}
		if *pick {
			if selected, err = pickRepos(discovered, nil); err != nil {
				return err
			}
		} else if len(orgs) == 0 && len(repos) == 0 {
			selected = discovered
		} else {
			sel := Profile{Orgs: orgs, Repos: repos}
			for _, r := range discovered {
				if sel.includes(r) {
					selected = append(selected, r)
				}
			}
		}
	case *pick:
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
	default:
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

	if *fetch {
		fmt.Fprintf(os.Stderr, "Fetching %d repos…\n", len(selected))
		fetchRepos(selected)
	}
	all := scanAll(selected, from, to, emails, *diffstat)

	recap := Recap{Profile: profileName, Name: name, From: from, To: to, Commits: all}
	fmt.Print(render(outFormat, recap))

	if *write {
		folder := cfg.recapsFolder()
		if *folderFlag != "" {
			folder = *folderFlag
		}
		path, err := writeJournal(folder, profileName, year, name, renderMarkdown(recap))
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Wrote %s — %d commits across %d repos.\n", path, len(all), len(selected))
	}
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
