package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Repo is a locally-cloned git repository.
type Repo struct {
	Path string // local filesystem path
	Org  string // owner/org from the origin remote ("" if none)
	Name string // repo name
}

// Slug is the "org/name" label used in config matching and markdown headings.
func (r Repo) Slug() string {
	if r.Org == "" {
		return r.Name
	}
	return r.Org + "/" + r.Name
}

// Commit is a single non-merge commit attributed to a scanned repo.
type Commit struct {
	Hash    string
	When    time.Time // author date, local
	Subject string
	Repo    Repo
}

// parseOrgRepo extracts owner and repo name from any git remote URL form:
// https://host/org/repo(.git), ssh://host/org/repo, git@host:org/repo(.git).
// Host-agnostic by design — no domain assumptions.
func parseOrgRepo(remote string) (org, name string) {
	s := strings.TrimSpace(remote)
	s = strings.TrimSuffix(s, ".git")

	if _, rest, found := strings.Cut(s, "://"); found {
		// scheme://[user@]host/org/repo -> strip scheme + host
		if _, path, ok := strings.Cut(rest, "/"); ok {
			s = path
		}
	} else if i := strings.Index(s, ":"); i >= 0 && strings.Contains(s[:i], "@") {
		// scp-like: [user@]host:org/repo -> keep path after ':'
		s = s[i+1:]
	}

	parts := strings.Split(strings.Trim(s, "/"), "/")
	switch n := len(parts); {
	case n >= 2:
		return parts[n-2], parts[n-1]
	case n == 1:
		return "", parts[0]
	default:
		return "", ""
	}
}

func repoRemote(path string) (string, error) {
	out, err := exec.Command("git", "-C", path, "remote", "get-url", "origin").Output()
	return strings.TrimSpace(string(out)), err
}

// discoverRepos walks the given roots and returns every git repo found,
// without descending into a repo once detected.
func discoverRepos(roots []string) []Repo {
	var repos []Repo
	seen := map[string]bool{}
	for _, root := range roots {
		root = expandTilde(root)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil // skip unreadable entries / non-dirs
			}
			if name := d.Name(); path != root && (strings.HasPrefix(name, ".") || name == "node_modules") {
				return fs.SkipDir // junk/hidden trees never hold *your* clones
			}
			if _, e := os.Stat(filepath.Join(path, ".git")); e != nil {
				return nil
			}
			if !seen[path] {
				seen[path] = true
				r := Repo{Path: path, Name: filepath.Base(path)}
				if remote, e := repoRemote(path); e == nil {
					if o, n := parseOrgRepo(remote); n != "" {
						r.Org, r.Name = o, n
					}
				}
				repos = append(repos, r)
			}
			return fs.SkipDir // don't walk inside a repo
		})
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Slug() < repos[j].Slug() })
	return repos
}

// scanCommits returns this repo's non-merge commits authored within [from, to)
// by any of the given emails. Empty emails means any author. --all walks every
// ref (local + remote branches, tags) so work on unmerged/unchecked-out
// branches is included.
//
// Date filtering: --since/--until filter by *commit* date, but we want *author*
// date. A rebase keeps the author date and moves the commit date forward —
// possibly by months — so an --until cutoff would silently drop rebased work.
// And plain --since *stops walking* at the first too-old commit, so one
// non-monotonic commit date (again: rebases) hides everything behind it.
// --since-as-filter (git ≥ 2.37) visits all commits; we pad it for clock skew
// (author date is never meaningfully after commit date) and filter precisely
// in Go. On older git we retry with no date limit at all — correct, just slower.
//
// Dedup: a rebased or cherry-picked commit reappears under a new hash on other
// refs. Same author email + author date + subject = the same work; keep one.
func scanCommits(repo Repo, from, to time.Time, emails []string) ([]Commit, error) {
	const sep = "\x1f"
	args := []string{
		"-C", repo.Path, "log", "--no-merges", "--all",
		"--pretty=format:%H" + sep + "%ae" + sep + "%aI" + sep + "%s",
	}
	for _, e := range emails {
		// Prefilter for speed only; exact matching happens on %ae below
		// (--author is an unanchored regex, so it can overmatch).
		args = append(args, "--author="+e)
	}
	sinceFilter := "--since-as-filter=" + from.Add(-36*time.Hour).Format(time.RFC3339)
	out, err := exec.Command("git", append(args, sinceFilter)...).Output()
	if err != nil {
		// git < 2.37: no --since-as-filter. Walk unfiltered; Go filters below.
		out, err = exec.Command("git", args...).Output()
	}
	if err != nil {
		return nil, err
	}

	var commits []Commit
	seen := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		f := strings.SplitN(line, sep, 4)
		if len(f) != 4 {
			continue
		}
		hash, email, date, subject := f[0], f[1], f[2], f[3]
		if len(emails) > 0 && !containsFold(emails, email) {
			continue
		}
		when, err := time.Parse(time.RFC3339, date)
		if err != nil {
			continue
		}
		when = when.Local()
		if when.Before(from) || !when.Before(to) {
			continue // precise author-date window, [from, to)
		}
		key := email + sep + date + sep + subject
		if seen[key] {
			continue // rebased/cherry-picked duplicate on another ref
		}
		seen[key] = true
		commits = append(commits, Commit{Hash: hash, When: when, Subject: subject, Repo: repo})
	}
	return commits, nil
}

// containsFold reports whether list contains s, case-insensitively.
func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
