package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// fixtureRepo builds a throwaway git repo for scanner tests.
type fixtureRepo struct {
	t    *testing.T
	path string
}

func newFixtureRepo(t *testing.T) fixtureRepo {
	t.Helper()
	r := fixtureRepo{t: t, path: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	r.git("config", "user.name", "Test")
	r.git("config", "user.email", "me@test.dev")
	return r
}

func (r fixtureRepo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.path}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// commit creates an empty commit with forced author/committer dates and email.
func (r fixtureRepo) commit(subject, email, authorDate, commitDate string) {
	r.t.Helper()
	cmd := exec.Command("git", "-C", r.path, "commit", "-q", "--allow-empty", "-m", subject)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL="+email, "GIT_AUTHOR_DATE="+authorDate,
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL="+email, "GIT_COMMITTER_DATE="+commitDate,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("commit %q: %v\n%s", subject, err, out)
	}
}

func TestScanCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	r := newFixtureRepo(t)
	loc := time.Local
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 7, 1, 0, 0, 0, 0, loc)

	// In-window commit, normal.
	r.commit("in window", "me@test.dev", "2026-06-10T10:00:00", "2026-06-10T10:00:00")
	// Authored in window but committed months later (late rebase) — must NOT be lost.
	r.commit("late rebase", "me@test.dev", "2026-06-15T09:00:00", "2026-09-20T17:00:00")
	// Another author — must be filtered out.
	r.commit("other author", "other@test.dev", "2026-06-16T09:00:00", "2026-06-16T09:00:00")
	// Overmatching email — --author regex would match me@test.devx; exact match must not.
	r.commit("email overmatch", "me@test.devx", "2026-06-17T09:00:00", "2026-06-17T09:00:00")
	// Boundary: exactly at `to` — excluded ([from, to)).
	r.commit("at to boundary", "me@test.dev", "2026-07-01T00:00:00", "2026-07-01T00:00:00")
	// Before the window.
	r.commit("too old", "me@test.dev", "2026-05-20T09:00:00", "2026-05-20T09:00:00")

	// Rebased duplicate: same subject + author date on a second branch with a
	// different commit date → different hash, reachable via --all. Dedup keeps one.
	r.git("checkout", "-q", "-b", "pre-rebase")
	r.commit("duplicated work", "me@test.dev", "2026-06-20T11:00:00", "2026-06-20T11:00:00")
	r.git("checkout", "-q", "main")
	r.commit("duplicated work", "me@test.dev", "2026-06-20T11:00:00", "2026-06-25T11:00:00")

	got, err := scanCommits(Repo{Path: r.path, Name: "fixture"}, from, to, []string{"me@test.dev"}, false)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{"in window": true, "late rebase": true, "duplicated work": true}
	if len(got) != len(want) {
		var subjects []string
		for _, c := range got {
			subjects = append(subjects, c.Subject)
		}
		t.Fatalf("got %d commits %v, want %d %v", len(got), subjects, len(want), want)
	}
	for _, c := range got {
		if !want[c.Subject] {
			t.Errorf("unexpected commit %q", c.Subject)
		}
	}
}

func TestScanCommitsEmptyEmailsMatchesAll(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	r := newFixtureRepo(t)
	r.commit("anyone", "whoever@test.dev", "2026-06-10T10:00:00", "2026-06-10T10:00:00")
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	got, err := scanCommits(Repo{Path: r.path, Name: "fixture"}, from, to, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Subject != "anyone" {
		t.Fatalf("got %v, want the one commit by any author", got)
	}
}

func TestScanCommitsDiffstat(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	r := newFixtureRepo(t)
	if err := os.WriteFile(filepath.Join(r.path, "a.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.git("add", "a.txt")
	cmd := exec.Command("git", "-C", r.path, "commit", "-q", "-m", "add lines")
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_DATE=2026-06-10T10:00:00", "GIT_COMMITTER_DATE=2026-06-10T10:00:00",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}

	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	got, err := scanCommits(Repo{Path: r.path, Name: "fixture"}, from, to, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d commits, want 1", len(got))
	}
	if got[0].Files != 1 || got[0].Adds != 2 || got[0].Dels != 0 {
		t.Errorf("diffstat = %d files +%d −%d, want 1 files +2 −0", got[0].Files, got[0].Adds, got[0].Dels)
	}
}

func TestParseShortstat(t *testing.T) {
	cases := []struct {
		in                string
		files, adds, dels int
	}{
		{" 3 files changed, 10 insertions(+), 2 deletions(-)", 3, 10, 2},
		{" 1 file changed, 1 insertion(+)", 1, 1, 0},
		{" 2 files changed, 5 deletions(-)", 2, 0, 5},
		{"", 0, 0, 0},
	}
	for _, c := range cases {
		f, a, d := parseShortstat(c.in)
		if f != c.files || a != c.adds || d != c.dels {
			t.Errorf("parseShortstat(%q) = %d/%d/%d, want %d/%d/%d", c.in, f, a, d, c.files, c.adds, c.dels)
		}
	}
}

func TestDiscoverReposSkipsJunkDirs(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	mkRepo := func(rel string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "-C", p, "init", "-q")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("init %s: %v\n%s", rel, err, out)
		}
	}
	mkRepo("real")
	mkRepo("node_modules/dep")   // inside junk dir — skipped
	mkRepo(".cache/hidden-repo") // inside hidden dir — skipped

	repos := discoverRepos([]string{root})
	if len(repos) != 1 || repos[0].Name != "real" {
		t.Fatalf("discovered %v, want just [real]", repos)
	}
}
