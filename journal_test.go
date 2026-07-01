package main

import (
	"testing"
	"time"
)

func TestParseOrgRepo(t *testing.T) {
	cases := []struct{ in, org, name string }{
		{"https://github.com/acme/widgets.git", "acme", "widgets"},
		{"git@github.com:acme/widgets.git", "acme", "widgets"},
		{"https://user@gitlab.com/team/sub/proj.git", "sub", "proj"},
		{"ssh://git@host.example.com/org/repo", "org", "repo"},
		{"/local/path/repo", "path", "repo"},
		{"repo", "", "repo"},
	}
	for _, c := range cases {
		org, name := parseOrgRepo(c.in)
		if org != c.org || name != c.name {
			t.Errorf("parseOrgRepo(%q) = %q/%q, want %q/%q", c.in, org, name, c.org, c.name)
		}
	}
}

func TestDefaultRangeAndFilename(t *testing.T) {
	loc := time.UTC
	// Wed 2026-06-30
	ref := time.Date(2026, 6, 30, 15, 0, 0, 0, loc)

	// month
	from, to := defaultRange("month", ref)
	if !from.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, loc)) || !to.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, loc)) {
		t.Errorf("month range = %v..%v", from, to)
	}
	if _, name := periodFilename("month", from); name != "2026-06" {
		t.Errorf("month filename = %q, want 2026-06", name)
	}

	// week: Monday-start. 2026-06-30 is a Tuesday -> week starts Mon 2026-06-29.
	from, to = defaultRange("week", ref)
	if from.Weekday() != time.Monday {
		t.Errorf("week should start Monday, got %v", from.Weekday())
	}
	if !from.Equal(time.Date(2026, 6, 29, 0, 0, 0, 0, loc)) || !to.Equal(time.Date(2026, 7, 6, 0, 0, 0, 0, loc)) {
		t.Errorf("week range = %v..%v", from, to)
	}
	if _, name := periodFilename("week", from); name != "2026-W27" {
		t.Errorf("week filename = %q, want 2026-W27", name)
	}

	// quarter: Q2 = Apr 1 .. Jul 1
	from, to = defaultRange("quarter", ref)
	if !from.Equal(time.Date(2026, 4, 1, 0, 0, 0, 0, loc)) || !to.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, loc)) {
		t.Errorf("quarter range = %v..%v", from, to)
	}
	if _, name := periodFilename("quarter", from); name != "2026-Q2" {
		t.Errorf("quarter filename = %q, want 2026-Q2", name)
	}
}

func TestResolveRangeCustom(t *testing.T) {
	from, to, _, _, err := resolveRange("month", "2026-06-01", "2026-06-15", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// --to is inclusive: end "2026-06-15" becomes the 16th 00:00 (exclusive).
	if from.Format("2006-01-02") != "2026-06-01" || to.Format("2006-01-02") != "2026-06-16" {
		t.Errorf("custom range = %v..%v, want 2026-06-01..2026-06-16(excl)", from, to)
	}
	if _, _, _, _, err := resolveRange("month", "2026-06-01", "", time.Now()); err == nil {
		t.Error("expected error when only --from is given")
	}
}

func TestResolveRangePresets(t *testing.T) {
	loc := time.UTC
	ref := time.Date(2026, 6, 30, 15, 0, 0, 0, loc) // Tue 2026-06-30

	cases := []struct{ period, name, from, to string }{
		{"yesterday", "2026-06-29", "2026-06-29", "2026-06-30"},
		{"last-week", "2026-W26", "2026-06-22", "2026-06-29"},
		{"last-month", "2026-05", "2026-05-01", "2026-06-01"},
		{"last-7-days", "2026-06-23_2026-06-29", "2026-06-23", "2026-06-30"},
		{"last-30-days", "2026-05-31_2026-06-29", "2026-05-31", "2026-06-30"},
	}
	for _, c := range cases {
		from, to, _, name, err := resolveRange(c.period, "", "", ref)
		if err != nil {
			t.Fatalf("%s: %v", c.period, err)
		}
		if from.Format("2006-01-02") != c.from || to.Format("2006-01-02") != c.to {
			t.Errorf("%s range = %s..%s, want %s..%s", c.period,
				from.Format("2006-01-02"), to.Format("2006-01-02"), c.from, c.to)
		}
		if name != c.name {
			t.Errorf("%s filename = %q, want %q", c.period, name, c.name)
		}
	}

	if _, _, _, _, err := resolveRange("nonsense", "", "", ref); err == nil {
		t.Error("expected error for unknown period token")
	}
}

func TestVersionStringLdflags(t *testing.T) {
	old := version
	defer func() { version = old }()
	version = "v9.9.9"
	if got := versionString(); got != "v9.9.9" {
		t.Errorf("versionString() = %q, want v9.9.9", got)
	}
}

func TestGroupByDay(t *testing.T) {
	mk := func(day, hh int) Commit {
		return Commit{When: time.Date(2026, 6, day, hh, 0, 0, 0, time.UTC), Repo: Repo{Name: "r"}}
	}
	commits := []Commit{mk(30, 9), mk(30, 14), mk(29, 10)}
	g := groupByDay(commits)
	if len(g["2026-06-30"]) != 2 || len(g["2026-06-29"]) != 1 {
		t.Errorf("groupByDay buckets = %v", map[string]int{"30": len(g["2026-06-30"]), "29": len(g["2026-06-29"])})
	}
}
