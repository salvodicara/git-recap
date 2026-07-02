package main

import (
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// parseJournal reads one journal markdown file back into commits. The format
// is our own renderMarkdown output — the roundtrip test pins them together.
// This is what lets `index` work even for periods whose repos are long gone.
var (
	dayRE  = regexp.MustCompile(`^## (\d{4}-\d{2}-\d{2})$`)
	repoRE = regexp.MustCompile(`^### (.+)$`)
	// ponytail: a subject literally ending in " (+N −M)" (U+2212 minus) is
	// indistinguishable from a diffstat suffix and parses as one. Inherent
	// format ambiguity; a metadata sidecar would be the precise upgrade.
	commitRE = regexp.MustCompile("^- `([0-9a-f]+)` (\\d{2}):(\\d{2}) — (.*?)( \\(\\+(\\d+) −(\\d+)\\))?$")
	yearRE   = regexp.MustCompile(`^\d{4}$`)
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
			hh, _ := strconv.Atoi(m[2])
			mm, _ := strconv.Atoi(m[3])
			c := Commit{
				Hash: m[1],
				// time.Date, not day.Add: a duration past midnight lands on
				// the wrong wall-clock time (or day) across DST transitions.
				When:    time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, time.Local),
				Subject: m[4],
				Repo:    repo,
			}
			if m[5] != "" {
				c.Adds, _ = strconv.Atoi(m[6])
				c.Dels, _ = strconv.Atoi(m[7])
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
		cfg, _, err := loadConfig()
		if err != nil {
			return fmt.Errorf("no --recaps-folder given and no usable config: %w", err)
		}
		folder = cfg.recapsFolder()
	}
	root := expandTilde(folder)

	type profileAgg struct {
		seen    map[string]bool           // day+hash — dedup across overlapping periods
		counts  map[string]int            // day → deduped commits
		periods map[string]map[string]int // folder year → period → commit count
	}
	aggs := map[string]*profileAgg{}
	pages := 0

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 3 || !yearRE.MatchString(parts[1]) { // <profile>/<year>/<period>.md
			return nil
		}
		profile, year, period := parts[0], parts[1], strings.TrimSuffix(parts[2], ".md")

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := stripFrontmatter(string(raw), profile, period)
		// Only our own journals — never touch (or overwrite the .html twin
		// of) other markdown the user keeps in the folder.
		if !strings.HasPrefix(content, "# "+profile+" — "+period+"\n") {
			return nil
		}
		commits := parseJournal(content)

		// Per-period page next to the journal, over the period's full window
		// (falling back to the observed span for unrecognized names).
		from, to := periodSpan(period)
		if from.IsZero() {
			from, to = journalSpan(commits)
		}
		page := renderHTML(Recap{Profile: profile, Name: period, From: from, To: to, Commits: commits})
		if err := os.WriteFile(strings.TrimSuffix(path, ".md")+".html", []byte(page), 0o644); err != nil {
			return err
		}
		pages++

		agg := aggs[profile]
		if agg == nil {
			agg = &profileAgg{seen: map[string]bool{}, counts: map[string]int{}, periods: map[string]map[string]int{}}
			aggs[profile] = agg
		}
		if agg.periods[year] == nil {
			agg.periods[year] = map[string]int{}
		}
		agg.periods[year][period] = len(commits)
		// Dedup the overlap between period files (a commit lives in its week
		// AND its month) profile-wide, so cross-year weeks can't double-count.
		for _, c := range commits {
			day := c.When.Format("2006-01-02")
			if key := day + c.Hash; !agg.seen[key] {
				agg.seen[key] = true
				agg.counts[day]++
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

	// Build the index page model. Heatmaps and totals go by each commit's own
	// calendar year (an ISO week can span two); period links by folder year.
	type indexPeriod struct {
		Name, Href string
		Commits    int
	}
	type indexYear struct {
		Year    string
		Total   int
		Heat    heatmapData
		Periods []indexPeriod
	}
	type indexProfile struct {
		Name  string
		Years []indexYear
	}
	var data struct{ Profiles []indexProfile }
	for _, name := range slices.Sorted(maps.Keys(aggs)) {
		agg := aggs[name]
		p := indexProfile{Name: name}
		years := map[string]bool{}
		for day := range agg.counts {
			years[day[:4]] = true
		}
		for y := range agg.periods {
			years[y] = true
		}
		for _, y := range slices.SortedFunc(maps.Keys(years), func(a, b string) int { return strings.Compare(b, a) }) {
			yearCounts := map[string]int{}
			total := 0
			for day, n := range agg.counts {
				if strings.HasPrefix(day, y+"-") {
					yearCounts[day] = n
					total += n
				}
			}
			jan1 := time.Date(mustAtoi(y), 1, 1, 0, 0, 0, 0, time.Local)
			iy := indexYear{Year: y, Total: total, Heat: buildHeatmap(jan1, jan1.AddDate(1, 0, 0), yearCounts)}
			for _, period := range slices.Sorted(maps.Keys(agg.periods[y])) {
				iy.Periods = append(iy.Periods, indexPeriod{
					Name: period, Href: name + "/" + y + "/" + period + ".html", Commits: agg.periods[y][period],
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

// mustAtoi converts digits already validated by yearRE / day formatting.
func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

var (
	quarterRE = regexp.MustCompile(`^(\d{4})-Q([1-4])$`)
	weekRE    = regexp.MustCompile(`^(\d{4})-W(\d{2})$`)
	rangeRE   = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})_(\d{4}-\d{2}-\d{2})$`)
)

// periodSpan derives the full [from, to) window from a period filename
// (2026-06-30, 2026-W27, 2026-06, 2026-Q2, 2026, from_to). Zero times mean
// the name isn't a recognized period.
func periodSpan(name string) (from, to time.Time) {
	loc := time.Local
	if t, err := time.ParseInLocation("2006-01-02", name, loc); err == nil {
		return t, t.AddDate(0, 0, 1)
	}
	if t, err := time.ParseInLocation("2006-01", name, loc); err == nil {
		return t, t.AddDate(0, 1, 0)
	}
	if t, err := time.ParseInLocation("2006", name, loc); err == nil {
		return t, t.AddDate(1, 0, 0)
	}
	if m := quarterRE.FindStringSubmatch(name); m != nil {
		q := mustAtoi(m[2])
		from = time.Date(mustAtoi(m[1]), time.Month(3*(q-1)+1), 1, 0, 0, 0, 0, loc)
		return from, from.AddDate(0, 3, 0)
	}
	if m := weekRE.FindStringSubmatch(name); m != nil {
		// ISO week 1 always contains Jan 4; back up to its Monday.
		jan4 := time.Date(mustAtoi(m[1]), 1, 4, 0, 0, 0, 0, loc)
		monday := jan4.AddDate(0, 0, -((int(jan4.Weekday()) + 6) % 7))
		from = monday.AddDate(0, 0, 7*(mustAtoi(m[2])-1))
		return from, from.AddDate(0, 0, 7)
	}
	if m := rangeRE.FindStringSubmatch(name); m != nil {
		from, _ = time.ParseInLocation("2006-01-02", m[1], loc)
		last, _ := time.ParseInLocation("2006-01-02", m[2], loc)
		return from, last.AddDate(0, 0, 1)
	}
	return time.Time{}, time.Time{}
}

// stripFrontmatter drops a leading YAML frontmatter block — but only one we
// wrote (it must carry this journal's own quoted profile and period), so a
// user's frontmattered notes stay opaque to the journal-heading guard.
func stripFrontmatter(s, profile, period string) string {
	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		return s
	}
	head, body, ok := strings.Cut(rest, "\n---\n")
	if !ok ||
		!strings.Contains(head, "\nprofile: "+yamlQuote(profile)) ||
		!strings.Contains(head, "\nperiod: "+yamlQuote(period)) {
		return s
	}
	return strings.TrimPrefix(body, "\n")
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
