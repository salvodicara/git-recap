package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testRecap() Recap {
	repo := Repo{Org: "acme", Name: "widgets"}
	loc := time.UTC
	return Recap{
		Profile: "work",
		Name:    "2026-06",
		From:    time.Date(2026, 6, 1, 0, 0, 0, 0, loc),
		To:      time.Date(2026, 7, 1, 0, 0, 0, 0, loc),
		Commits: []Commit{
			{Hash: "aaaaaaaaaaaa", When: time.Date(2026, 6, 30, 14, 32, 0, 0, loc), Subject: "Fix parser", Repo: repo},
			{Hash: "bbbbbbbbbbbb", When: time.Date(2026, 6, 30, 9, 15, 0, 0, loc), Subject: "Add retry", Repo: repo},
		},
	}
}

func TestRenderMarkdown(t *testing.T) {
	got := renderMarkdown(testRecap())
	for _, want := range []string{
		"# work — 2026-06",
		"## 2026-06-30",
		"### acme/widgets",
		"- `bbbbbbb` 09:15 — Add retry",
		"- `aaaaaaa` 14:32 — Fix parser",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown missing %q in:\n%s", want, got)
		}
	}
	// Commits must be time-ordered within a repo.
	if strings.Index(got, "Add retry") > strings.Index(got, "Fix parser") {
		t.Error("commits not time-ordered")
	}

	empty := renderMarkdown(Recap{Profile: "work", Name: "2026-06"})
	if !strings.Contains(empty, "No commits") {
		t.Errorf("empty recap should say so, got:\n%s", empty)
	}
}

func TestRenderJSON(t *testing.T) {
	var out struct {
		Profile string    `json:"profile"`
		Period  string    `json:"period"`
		From    time.Time `json:"from"`
		To      time.Time `json:"to"`
		Commits []struct {
			Hash    string    `json:"hash"`
			Date    time.Time `json:"date"`
			Subject string    `json:"subject"`
			Repo    string    `json:"repo"`
		} `json:"commits"`
	}
	if err := json.Unmarshal([]byte(renderJSON(testRecap())), &out); err != nil {
		t.Fatal(err)
	}
	if out.Profile != "work" || out.Period != "2026-06" || len(out.Commits) != 2 {
		t.Fatalf("unexpected JSON: %+v", out)
	}
	if out.Commits[0].Subject != "Add retry" || out.Commits[0].Repo != "acme/widgets" {
		t.Errorf("commits not time-ordered or mis-slugged: %+v", out.Commits)
	}

	// Empty commits must encode as [], not null — scripts depend on it.
	if s := renderJSON(Recap{}); strings.Contains(s, "null") {
		t.Errorf("empty commits rendered as null:\n%s", s)
	}
}

func TestRenderTerm(t *testing.T) {
	got := renderTerm(testRecap())
	for _, want := range []string{"acme/widgets", "Add retry", "Fix parser", "2 commits · 1 repos"} {
		if !strings.Contains(got, want) {
			t.Errorf("term output missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderDispatch(t *testing.T) {
	if _, err := render("bogus", testRecap()); err == nil {
		t.Error("expected error for unknown format")
	}
	for _, f := range []string{"term", "md", "markdown", "json"} {
		if _, err := render(f, testRecap()); err != nil {
			t.Errorf("render(%q): %v", f, err)
		}
	}
}
