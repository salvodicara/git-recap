package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestJournalRoundtrip pins parseJournal to renderMarkdown: whatever the
// renderer writes, the parser must read back identically.
func TestJournalRoundtrip(t *testing.T) {
	repo := Repo{Org: "acme", Name: "widgets"}
	loc := time.Local
	in := []Commit{
		{Hash: "aaaaaaaaaaaa", When: time.Date(2026, 6, 29, 9, 15, 0, 0, loc), Subject: "Add retry — with a dash", Repo: repo},
		{Hash: "bbbbbbbbbbbb", When: time.Date(2026, 6, 30, 14, 32, 0, 0, loc), Subject: "Fix parser", Repo: repo, Files: 2, Adds: 10, Dels: 3},
		{Hash: "cccccccccccc", When: time.Date(2026, 6, 30, 15, 0, 0, 0, loc), Subject: "Local repo commit", Repo: Repo{Name: "scratch"}},
	}
	md := renderMarkdown(Recap{Profile: "work", Name: "2026-06", Commits: in})
	out := parseJournal(md)
	if len(out) != len(in) {
		t.Fatalf("roundtrip: got %d commits, want %d\n%s", len(out), len(in), md)
	}
	for i, c := range out {
		want := in[i]
		if c.Hash != shortHash(want.Hash) || c.Subject != want.Subject ||
			c.When.Format("2006-01-02 15:04") != want.When.Format("2006-01-02 15:04") ||
			c.Repo.Slug() != want.Repo.Slug() || c.Adds != want.Adds || c.Dels != want.Dels {
			t.Errorf("roundtrip[%d] = %+v, want %+v", i, c, want)
		}
	}
}

func TestRunIndex(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "work", "2026")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	repo := Repo{Org: "acme", Name: "widgets"}
	commits := []Commit{
		{Hash: "aaaaaaaaaaaa", When: time.Date(2026, 6, 29, 9, 15, 0, 0, time.Local), Subject: "Add retry", Repo: repo},
	}
	md := renderMarkdown(Recap{Profile: "work", Name: "2026-06", Commits: commits})
	if err := os.WriteFile(filepath.Join(dir, "2026-06.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	// A week file overlapping the month — the year heatmap must not double-count.
	// Written with frontmatter: index must still recognize it as a journal.
	wk := renderMarkdown(Recap{Profile: "work", Name: "2026-W27", Commits: commits, Frontmatter: true})
	if err := os.WriteFile(filepath.Join(dir, "2026-W27.md"), []byte(wk), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runIndex([]string{"--recaps-folder", root}); err != nil {
		t.Fatal(err)
	}

	idx, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"work", "2026 · 1 commit", `href="work/2026/2026-06.html"`, "2026-W27"} {
		if !strings.Contains(string(idx), want) {
			t.Errorf("index.html missing %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-06.html")); err != nil {
		t.Error("per-period page not written next to the journal")
	}
	// The regenerated month page must cover the whole month, not just the
	// days that had commits: June 1 renders as an (empty) in-range cell.
	page, err := os.ReadFile(filepath.Join(dir, "2026-06.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `title="2026-06-01 · 0 commits"`) {
		t.Error("period page heatmap doesn't span the full month window")
	}
}

func TestRunIndexNeverTouchesForeignFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "work", "2026")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A real journal so the run succeeds…
	md := renderMarkdown(Recap{Profile: "work", Name: "2026-06", Commits: []Commit{
		{Hash: "aaaaaaaaaaaa", When: time.Date(2026, 6, 29, 9, 0, 0, 0, time.Local), Subject: "x", Repo: Repo{Name: "r"}},
	}})
	if err := os.WriteFile(filepath.Join(dir, "2026-06.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	// …and the user's own notes with a hand-made .html twin.
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# my private notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.html"), []byte("PRECIOUS"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A frontmattered note whose body mimics a journal heading: foreign
	// frontmatter must NOT be stripped, so the file stays untouched.
	crafted := "---\nfoo: bar\n---\n\n# work — crafted\n"
	if err := os.WriteFile(filepath.Join(dir, "crafted.md"), []byte(crafted), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "crafted.html"), []byte("ALSO PRECIOUS"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runIndex([]string{"--recaps-folder", root}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "notes.html"))
	if err != nil || string(got) != "PRECIOUS" {
		t.Errorf("notes.html was overwritten: %q, %v", got, err)
	}
	got, err = os.ReadFile(filepath.Join(dir, "crafted.html"))
	if err != nil || string(got) != "ALSO PRECIOUS" {
		t.Errorf("crafted.html was overwritten: %q, %v", got, err)
	}
	if idx, _ := os.ReadFile(filepath.Join(root, "index.html")); strings.Contains(string(idx), "notes") || strings.Contains(string(idx), "crafted") {
		t.Error("foreign markdown leaked into the index")
	}
}

func TestRunIndexCrossYearWeek(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "work", "2026")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// ISO week 2026-W01 files under 2026 but this commit is on 2025-12-30:
	// it must count toward 2025, not 2026.
	md := renderMarkdown(Recap{Profile: "work", Name: "2026-W01", Commits: []Commit{
		{Hash: "aaaaaaaaaaaa", When: time.Date(2025, 12, 30, 9, 0, 0, 0, time.Local), Subject: "x", Repo: Repo{Name: "r"}},
	}})
	if err := os.WriteFile(filepath.Join(dir, "2026-W01.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runIndex([]string{"--recaps-folder", root}); err != nil {
		t.Fatal(err)
	}
	idx, _ := os.ReadFile(filepath.Join(root, "index.html"))
	if !strings.Contains(string(idx), "2025 · 1 commit") {
		t.Error("cross-year week commit not attributed to its own calendar year")
	}
	if strings.Contains(string(idx), "2026 · 1 commit") {
		t.Error("cross-year week commit double-counted into the folder year")
	}
}

func TestPeriodSpan(t *testing.T) {
	loc := time.Local
	cases := []struct{ name, from, to string }{
		{"2026-06-30", "2026-06-30", "2026-07-01"},
		{"2026-06", "2026-06-01", "2026-07-01"},
		{"2026", "2026-01-01", "2027-01-01"},
		{"2026-Q2", "2026-04-01", "2026-07-01"},
		{"2026-W01", "2025-12-29", "2026-01-05"}, // ISO week 1 spans years
		{"2026-W27", "2026-06-29", "2026-07-06"},
		{"2026-05-03_2026-05-19", "2026-05-03", "2026-05-20"},
	}
	for _, c := range cases {
		from, to := periodSpan(c.name)
		if from.Format("2006-01-02") != c.from || to.Format("2006-01-02") != c.to {
			t.Errorf("periodSpan(%q) = %s..%s, want %s..%s", c.name,
				from.Format("2006-01-02"), to.Format("2006-01-02"), c.from, c.to)
		}
	}
	if from, _ := periodSpan("not-a-period"); !from.IsZero() {
		t.Error("unrecognized name should return zero times")
	}
	_ = loc
}
