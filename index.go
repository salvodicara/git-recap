package main

import (
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// parseJournal reads one journal markdown file back into commits. The format
// is our own renderMarkdown output — the roundtrip test pins them together.
// This is what lets `index` work even for periods whose repos are long gone.
var (
	dayRE    = regexp.MustCompile(`^## (\d{4}-\d{2}-\d{2})$`)
	repoRE   = regexp.MustCompile(`^### (.+)$`)
	commitRE = regexp.MustCompile("^- `([0-9a-f]+)` (\\d{2}:\\d{2}) — (.*?)( \\(\\+(\\d+) −(\\d+)\\))?$")
)

func parseJournal(content string) []Commit {
	var (
		commits []Commit
		day     time.Time
		repo    Repo
	)
	for line := range strings.SplitSeq(content, "\n") {
		if m := dayRE.FindStringSubmatch(line); m != nil {
			day, _ = time.ParseInLocation("2006-01-02", m[1], time.Local)
			continue
		}
		if m := repoRE.FindStringSubmatch(line); m != nil {
			org, name, _ := strings.Cut(m[1], "/")
			if name == "" {
				org, name = "", org
			}
			repo = Repo{Org: org, Name: name}
			continue
		}
		if m := commitRE.FindStringSubmatch(line); m != nil && !day.IsZero() {
			hh, _ := time.Parse("15:04", m[2])
			c := Commit{
				Hash:    m[1],
				When:    day.Add(time.Duration(hh.Hour())*time.Hour + time.Duration(hh.Minute())*time.Minute),
				Subject: m[3],
				Repo:    repo,
			}
			if m[4] != "" {
				fmt.Sscanf(m[5], "%d", &c.Adds)
				fmt.Sscanf(m[6], "%d", &c.Dels)
				c.Files = 1 // stat line implies files changed; exact count isn't journaled
			}
			commits = append(commits, c)
		}
	}
	return commits
}

const indexUsage = `git-recap index — rebuild index.html and per-period pages in the recaps folder

Reads every journal (<profile>/<year>/<period>.md), writes an .html page next
to each, and an index.html at the root: per-year contribution heatmaps and
totals per profile. Push the folder to any static host and it's a site.

  --recaps-folder PATH   index this folder instead of the configured one`

// runIndex regenerates the static site for the recaps folder.
func runIndex(argv []string) error {
	fs := flag.NewFlagSet("git-recap index", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, indexUsage) }
	folderFlag := fs.String("recaps-folder", "", "recaps folder to index")
	if err := fs.Parse(argv); err != nil {
		return err
	}

	folder := *folderFlag
	if folder == "" {
		cfg, cfgPath, err := loadConfig()
		if err != nil {
			return fmt.Errorf("no --recaps-folder given and no config at %s", cfgPath)
		}
		folder = cfg.recapsFolder()
	}
	root := expandTilde(folder)

	type yearAgg struct {
		counts  map[string]int  // day → commits (deduped)
		seen    map[string]bool // day+hash, dedup across overlapping periods
		periods map[string]int  // period name → commit count
	}
	profiles := map[string]map[string]*yearAgg{} // profile → year → agg
	pages := 0

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 3 { // <profile>/<year>/<period>.md
			return nil
		}
		profile, year, period := parts[0], parts[1], strings.TrimSuffix(parts[2], ".md")

		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		commits := parseJournal(string(raw))

		// Per-period page next to the journal.
		from, to := journalSpan(commits)
		page := renderHTML(Recap{Profile: profile, Name: period, From: from, To: to, Commits: commits})
		if err := os.WriteFile(strings.TrimSuffix(path, ".md")+".html", []byte(page), 0o644); err != nil {
			return err
		}
		pages++

		// Aggregate into the profile/year heatmap, deduping the overlap
		// between period files (a commit lives in its week AND its month).
		years := profiles[profile]
		if years == nil {
			years = map[string]*yearAgg{}
			profiles[profile] = years
		}
		agg := years[year]
		if agg == nil {
			agg = &yearAgg{counts: map[string]int{}, seen: map[string]bool{}, periods: map[string]int{}}
			years[year] = agg
		}
		agg.periods[period] = len(commits)
		for _, c := range commits {
			key := c.When.Format("2006-01-02") + c.Hash
			if !agg.seen[key] {
				agg.seen[key] = true
				agg.counts[c.When.Format("2006-01-02")]++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if pages == 0 {
		return fmt.Errorf("no journals found under %s — generate some with --write first", root)
	}

	// Build the index page model.
	type indexPeriod struct {
		Name, Href string
		Commits    int
	}
	type indexYear struct {
		Year    string
		Total   int
		Weeks   [][]heatCell
		Periods []indexPeriod
	}
	type indexProfile struct {
		Name  string
		Years []indexYear
	}
	var data struct{ Profiles []indexProfile }
	for _, name := range slices.Sorted(maps.Keys(profiles)) {
		p := indexProfile{Name: name}
		years := profiles[name]
		for _, y := range slices.SortedFunc(maps.Keys(years), func(a, b string) int { return strings.Compare(b, a) }) {
			agg := years[y]
			jan1, _ := time.ParseInLocation("2006", y, time.Local)
			iy := indexYear{Year: y, Weeks: buildWeeks(jan1, jan1.AddDate(1, 0, 0), agg.counts)}
			for _, n := range agg.counts {
				iy.Total += n
			}
			for _, period := range slices.Sorted(maps.Keys(agg.periods)) {
				iy.Periods = append(iy.Periods, indexPeriod{
					Name: period, Href: name + "/" + y + "/" + period + ".html", Commits: agg.periods[period],
				})
			}
			p.Years = append(p.Years, iy)
		}
		data.Profiles = append(data.Profiles, p)
	}

	var b strings.Builder
	if err := htmlTmpl.ExecuteTemplate(&b, "index", data); err != nil {
		panic(err) // static template; a failure is a programming bug
	}
	out := filepath.Join(root, "index.html")
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("Wrote %s + %d period pages.\n", out, pages)
	return nil
}

// journalSpan is the [first day, last day+1) window covered by parsed commits.
func journalSpan(commits []Commit) (from, to time.Time) {
	for _, c := range commits {
		if from.IsZero() || c.When.Before(from) {
			from = c.When
		}
		if c.When.After(to) {
			to = c.When
		}
	}
	if from.IsZero() {
		return time.Time{}, time.Time{}
	}
	y, m, d := from.Date()
	from = time.Date(y, m, d, 0, 0, 0, 0, from.Location())
	y, m, d = to.Date()
	return from, time.Date(y, m, d, 0, 0, 0, 0, to.Location()).AddDate(0, 0, 1)
}
