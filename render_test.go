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
		Stats   struct {
			Commits    int `json:"commits"`
			ActiveDays int `json:"active_days"`
		} `json:"stats"`
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
	if out.Stats.Commits != 2 || out.Stats.ActiveDays != 1 {
		t.Errorf("stats = %+v, want 2 commits / 1 active day", out.Stats)
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
	for _, want := range []string{"acme/widgets", "Add retry", "Fix parser", "2 commits · 1 repo · 1 active day"} {
		if !strings.Contains(got, want) {
			t.Errorf("term output missing %q in:\n%s", want, got)
		}
	}
}

func TestRecapStats(t *testing.T) {
	r := testRecap()
	other := Repo{Org: "acme", Name: "api"}
	r.Commits = append(r.Commits,
		Commit{Hash: "cc", When: time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC), Subject: "x", Repo: other, Adds: 10, Dels: 2})
	s := r.Stats()
	if s.Commits != 3 || s.Repos != 2 || s.ActiveDays != 2 {
		t.Errorf("stats = %+v, want 3 commits, 2 repos, 2 active days", s)
	}
	if s.Busiest != "acme/widgets (2)" {
		t.Errorf("busiest = %q, want acme/widgets (2)", s.Busiest)
	}
	if s.Adds != 10 || s.Dels != 2 {
		t.Errorf("lines = +%d −%d, want +10 −2", s.Adds, s.Dels)
	}
	if got := s.summary(); !strings.Contains(got, "busiest: acme/widgets (2)") || !strings.Contains(got, "+10 −2") {
		t.Errorf("summary = %q", got)
	}
}

func TestRenderHTML(t *testing.T) {
	r := testRecap()
	r.Commits[0].Subject = `Fix <script>alert("xss")</script> parser`
	got := renderHTML(r)
	for _, want := range []string{
		"<!doctype html>",
		"work — 2026-06",
		`class="l4"`, // busiest day gets the top intensity level
		"2026-06-30 · Tuesday",
		"acme/widgets",
		"Fix &lt;script&gt;", // subjects are escaped
	} {
		if !strings.Contains(got, want) {
			t.Errorf("html missing %q", want)
		}
	}
	if strings.Contains(got, "<script>alert") {
		t.Error("unescaped subject in HTML output")
	}
	if strings.Contains(got, "http") {
		t.Error("html should be self-contained — no external URLs")
	}

	if empty := renderHTML(Recap{Profile: "work", Name: "2026-06",
		From: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)}); !strings.Contains(empty, "No commits") {
		t.Error("empty recap should say so")
	}
}

func TestRenderDispatch(t *testing.T) {
	for _, f := range []string{"term", "md", "markdown", "json", "html"} {
		if render(f, testRecap()) == "" {
			t.Errorf("render(%q) produced no output", f)
		}
	}
}
