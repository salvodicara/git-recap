package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Recap is the one data model every renderer consumes. Features land here,
// never inside a single renderer — if it isn't visible in JSON, it doesn't exist.
type Recap struct {
	Profile     string
	Name        string // resolved period label, doubles as the filename base
	From        time.Time
	To          time.Time // exclusive
	Commits     []Commit
	Frontmatter bool // prepend YAML frontmatter to markdown (Obsidian-friendly)
}

// RecapStats are aggregates derived from the commits — computed, never stored.
// Insertions/deletions are always present (0 unless scanned with --diffstat)
// so scripts never have to distinguish "absent" from "zero".
type RecapStats struct {
	Commits    int    `json:"commits"`
	Repos      int    `json:"repos"`
	ActiveDays int    `json:"active_days"`
	Busiest    string `json:"busiest_repo,omitempty"` // slug
	BusiestN   int    `json:"busiest_repo_commits,omitempty"`
	Adds       int    `json:"insertions"`
	Dels       int    `json:"deletions"`
}

// Stats aggregates the recap's commits per repo and per day.
func (r Recap) Stats() RecapStats {
	s := RecapStats{Commits: len(r.Commits)}
	perRepo := map[string]int{}
	days := map[string]bool{}
	for _, c := range r.Commits {
		perRepo[c.Repo.Slug()]++
		days[c.When.Format("2006-01-02")] = true
		s.Adds += c.Adds
		s.Dels += c.Dels
	}
	s.Repos, s.ActiveDays = len(perRepo), len(days)
	best, bestN := "", 0
	for slug, n := range perRepo {
		if n > bestN || (n == bestN && slug < best) { // deterministic tie-break
			best, bestN = slug, n
		}
	}
	if bestN > 0 {
		s.Busiest, s.BusiestN = best, bestN
	}
	return s
}

// summary is the one-line human form of the stats, shared by term and md.
func (s RecapStats) summary() string {
	parts := []string{plural(s.Commits, "commit"), plural(s.Repos, "repo"), plural(s.ActiveDays, "active day")}
	if s.Repos > 1 {
		parts = append(parts, fmt.Sprintf("busiest: %s (%d)", s.Busiest, s.BusiestN))
	}
	if s.Adds+s.Dels > 0 {
		parts = append(parts, fmt.Sprintf("+%d −%d", s.Adds, s.Dels))
	}
	return strings.Join(parts, " · ")
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// groupByDay buckets commits by local calendar day (YYYY-MM-DD).
func groupByDay(commits []Commit) map[string][]Commit {
	byDay := map[string][]Commit{}
	for _, c := range commits {
		k := c.When.Format("2006-01-02")
		byDay[k] = append(byDay[k], c)
	}
	return byDay
}

func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

// statSuffix renders " (+adds −dels)" for commits scanned with --diffstat.
func statSuffix(c Commit) string {
	if c.Files == 0 {
		return ""
	}
	return fmt.Sprintf(" (+%d −%d)", c.Adds, c.Dels)
}

// byRepo groups one day's commits per repo slug, each repo's commits time-ordered.
func byRepo(commits []Commit) map[string][]Commit {
	m := map[string][]Commit{}
	for _, c := range commits {
		m[c.Repo.Slug()] = append(m[c.Repo.Slug()], c)
	}
	for _, cs := range m {
		sort.Slice(cs, func(i, j int) bool { return cs[i].When.Before(cs[j].When) })
	}
	return m
}

// renderMarkdown produces the journal: days ascending, repos grouped per day,
// commits time-ordered. This is also what --write persists.
func renderMarkdown(r Recap) string {
	var b strings.Builder
	if r.Frontmatter {
		fmt.Fprintf(&b, "---\ntitle: %s — %s\nprofile: %s\nperiod: %s\nfrom: %s\nto: %s\ncommits: %d\n---\n\n",
			r.Profile, r.Name, r.Profile, r.Name,
			r.From.Format("2006-01-02"), r.To.AddDate(0, 0, -1).Format("2006-01-02"), len(r.Commits))
	}
	fmt.Fprintf(&b, "# %s — %s\n", r.Profile, r.Name)
	if len(r.Commits) == 0 {
		b.WriteString("\n_No commits in this period._\n")
		return b.String()
	}
	fmt.Fprintf(&b, "\n_%s_\n", r.Stats().summary())
	byDay := groupByDay(r.Commits)
	for _, day := range slices.Sorted(maps.Keys(byDay)) {
		fmt.Fprintf(&b, "\n## %s\n", day)
		repos := byRepo(byDay[day])
		for _, slug := range slices.Sorted(maps.Keys(repos)) {
			fmt.Fprintf(&b, "\n### %s\n\n", slug)
			for _, c := range repos[slug] {
				fmt.Fprintf(&b, "- `%s` %s — %s%s\n", shortHash(c.Hash), c.When.Format("15:04"), c.Subject, statSuffix(c))
			}
		}
	}
	return b.String()
}

var (
	styleTitle = lipgloss.NewStyle().Faint(true)
	styleDay   = lipgloss.NewStyle().Bold(true)
	styleRepo  = lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan
	styleMeta  = lipgloss.NewStyle().Faint(true)
)

// renderTerm is the human view for a terminal: same structure as the markdown,
// styled and compact. Colors degrade automatically (termenv honours NO_COLOR
// and non-TTY output).
func renderTerm(r Recap) string {
	var b strings.Builder
	fmt.Fprintln(&b, styleTitle.Render(fmt.Sprintf("%s — %s", r.Profile, r.Name)))
	if len(r.Commits) == 0 {
		fmt.Fprintln(&b, "No commits in this period.")
		return b.String()
	}
	byDay := groupByDay(r.Commits)
	for _, day := range slices.Sorted(maps.Keys(byDay)) {
		t, _ := time.Parse("2006-01-02", day)
		fmt.Fprintf(&b, "\n%s\n", styleDay.Render(day+" · "+t.Format("Monday")))
		repos := byRepo(byDay[day])
		for _, slug := range slices.Sorted(maps.Keys(repos)) {
			fmt.Fprintf(&b, "  %s\n", styleRepo.Render(slug))
			for _, c := range repos[slug] {
				meta := styleMeta.Render(shortHash(c.Hash) + " " + c.When.Format("15:04"))
				fmt.Fprintf(&b, "    %s  %s%s\n", meta, c.Subject, styleMeta.Render(statSuffix(c)))
			}
		}
	}
	fmt.Fprintf(&b, "\n%s\n", styleMeta.Render(r.Stats().summary()))
	return b.String()
}

// renderJSON is the machine view — and the contract check for the data model.
func renderJSON(r Recap) string {
	type jsonCommit struct {
		Hash    string    `json:"hash"`
		Date    time.Time `json:"date"`
		Subject string    `json:"subject"`
		Repo    string    `json:"repo"`
		Files   int       `json:"files_changed,omitempty"`
		Adds    int       `json:"insertions,omitempty"`
		Dels    int       `json:"deletions,omitempty"`
	}
	out := struct {
		Profile string       `json:"profile"`
		Period  string       `json:"period"`
		From    time.Time    `json:"from"`
		To      time.Time    `json:"to"` // exclusive
		Stats   RecapStats   `json:"stats"`
		Commits []jsonCommit `json:"commits"`
	}{r.Profile, r.Name, r.From, r.To, r.Stats(), make([]jsonCommit, 0, len(r.Commits))}

	cs := append([]Commit(nil), r.Commits...)
	sort.Slice(cs, func(i, j int) bool { return cs[i].When.Before(cs[j].When) })
	for _, c := range cs {
		out.Commits = append(out.Commits, jsonCommit{c.Hash, c.When, c.Subject, c.Repo.Slug(), c.Files, c.Adds, c.Dels})
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b) + "\n"
}

// render dispatches on format; main validates the token, so anything not
// term/json/html is markdown.
func render(format string, r Recap) string {
	switch format {
	case "term":
		return renderTerm(r)
	case "json":
		return renderJSON(r)
	case "html":
		return renderHTML(r)
	default:
		return renderMarkdown(r)
	}
}
