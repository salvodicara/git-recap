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
// by any of the given emails. Empty emails means any author.
func scanCommits(repo Repo, from, to time.Time, emails []string) ([]Commit, error) {
	const sep = "\x1f"
	// ponytail: --since/--until filter by *commit* date; we want *author* date,
	// so pad the git window and filter precisely in Go below. Avoids dropping a
	// commit authored in-range but committed just outside it (rebases, late pushes).
	args := []string{
		"-C", repo.Path, "log", "--no-merges",
		"--since=" + from.Add(-36*time.Hour).Format(time.RFC3339),
		"--until=" + to.Add(36*time.Hour).Format(time.RFC3339),
		"--pretty=format:%H" + sep + "%aI" + sep + "%s",
	}
	for _, e := range emails {
		args = append(args, "--author="+e)
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, err
	}

	var commits []Commit
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, sep, 3)
		if len(f) != 3 {
			continue
		}
		when, err := time.Parse(time.RFC3339, f[1])
		if err != nil {
			continue
		}
		when = when.Local()
		if when.Before(from) || !when.Before(to) {
			continue // precise author-date window, [from, to)
		}
		commits = append(commits, Commit{Hash: f[0], When: when, Subject: f[2], Repo: repo})
	}
	return commits, nil
}
