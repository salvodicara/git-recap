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
	wk := renderMarkdown(Recap{Profile: "work", Name: "2026-W27", Commits: commits})
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
	for _, want := range []string{"work", "2026 · 1 commits", `href="work/2026/2026-06.html"`, "2026-W27"} {
		if !strings.Contains(string(idx), want) {
			t.Errorf("index.html missing %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-06.html")); err != nil {
		t.Error("per-period page not written next to the journal")
	}
}
