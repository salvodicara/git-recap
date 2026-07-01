package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Recap is the one data model every renderer consumes. Features land here,
// never inside a single renderer — if it isn't visible in JSON, it doesn't exist.
type Recap struct {
	Profile string
	Name    string // resolved period label, doubles as the filename base
	From    time.Time
	To      time.Time // exclusive
	Commits []Commit
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

func sortedKeys(m map[string][]Commit) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
	fmt.Fprintf(&b, "# %s — %s\n", r.Profile, r.Name)
	if len(r.Commits) == 0 {
		b.WriteString("\n_No commits in this period._\n")
		return b.String()
	}
	byDay := groupByDay(r.Commits)
	for _, day := range sortedKeys(byDay) {
		fmt.Fprintf(&b, "\n## %s\n", day)
		repos := byRepo(byDay[day])
		for _, slug := range sortedKeys(repos) {
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
	for _, day := range sortedKeys(byDay) {
		t, _ := time.Parse("2006-01-02", day)
		fmt.Fprintf(&b, "\n%s\n", styleDay.Render(day+" · "+t.Format("Monday")))
		repos := byRepo(byDay[day])
		for _, slug := range sortedKeys(repos) {
			fmt.Fprintf(&b, "  %s\n", styleRepo.Render(slug))
			for _, c := range repos[slug] {
				meta := styleMeta.Render(shortHash(c.Hash) + " " + c.When.Format("15:04"))
				fmt.Fprintf(&b, "    %s  %s%s\n", meta, c.Subject, styleMeta.Render(statSuffix(c)))
			}
		}
	}
	repoSet := map[string]bool{}
	for _, c := range r.Commits {
		repoSet[c.Repo.Slug()] = true
	}
	fmt.Fprintf(&b, "\n%s\n", styleMeta.Render(fmt.Sprintf("%d commits · %d repos", len(r.Commits), len(repoSet))))
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
		Commits []jsonCommit `json:"commits"`
	}{r.Profile, r.Name, r.From, r.To, make([]jsonCommit, 0, len(r.Commits))}

	cs := append([]Commit(nil), r.Commits...)
	sort.Slice(cs, func(i, j int) bool { return cs[i].When.Before(cs[j].When) })
	for _, c := range cs {
		out.Commits = append(out.Commits, jsonCommit{c.Hash, c.When, c.Subject, c.Repo.Slug(), c.Files, c.Adds, c.Dels})
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b) + "\n"
}

// render dispatches on format: "term", "md", or "json".
func render(format string, r Recap) (string, error) {
	switch format {
	case "term":
		return renderTerm(r), nil
	case "md", "markdown":
		return renderMarkdown(r), nil
	case "json":
		return renderJSON(r), nil
	default:
		return "", fmt.Errorf("invalid --format %q (term|md|json)", format)
	}
}
