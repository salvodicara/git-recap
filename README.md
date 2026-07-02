# git-recap

> What did I do since standup? Last month? All year? Your git history knows —
> `git recap` answers instantly, and can keep a journal you never have to write.

[![CI](https://github.com/salvodicara/git-recap/actions/workflows/ci.yml/badge.svg)](https://github.com/salvodicara/git-recap/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/salvodicara/git-recap)](https://github.com/salvodicara/git-recap/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/salvodicara/git-recap.svg)](https://pkg.go.dev/github.com/salvodicara/git-recap)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

No daemon, no external service, no API keys. Git already stores every commit;
`git-recap` reads it — across **all branches** of **all your repos** — and
turns it into a standup answer, a markdown journal, an HTML heatmap, or JSON.
(The binary is named `git-recap`, so `git recap` works too.)

![git recap in a terminal: one command prints the last working day's commits grouped by day and repo, with a stats summary](demo.gif)

- **Zero config.** Run it in any directory: everything since your last working
  day (Monday reaches back to Friday), using your `git config user.email`.
- **Nothing missed, nothing doubled.** All local and remote branches — unmerged
  work shows up; rebased and cherry-picked duplicates fold into one.
- **Any period.** `--period week`, `last-month`, `year`, or `--from`/`--to`.
  Backfill years of journal on day one; output is idempotent.
- **For humans and machines.** Styled terminal output (honours `NO_COLOR`),
  markdown when piped, `--format json|html` for everything else.

## Why not git-standup / the GitHub activity page?

[`git-standup`](https://github.com/kamranahmedse/git-standup) is great at one
moment: yesterday, in the terminal. GitHub's activity page only knows what you
pushed to GitHub.

|                                          | git-recap | git-standup |
|------------------------------------------|:---------:|:-----------:|
| Zero-config terminal standup             | ✓         | ✓           |
| All branches (unmerged work shows up)    | ✓         | current/one |
| Rebase/cherry-pick dedup                 | ✓         | –           |
| Calendar periods, custom ranges, backfill| ✓         | –           |
| Profiles (work / personal / OSS)         | ✓         | –           |
| Persistent markdown journal              | ✓         | –           |
| JSON / HTML output, heatmaps             | ✓         | –           |
| Actively maintained                      | ✓         | last release 2020 |

## Install

**Homebrew** (macOS/Linux):

```sh
brew install salvodicara/tap/git-recap
```

**curl** — prebuilt binary, checksum-verified:

```sh
curl -fsSL https://raw.githubusercontent.com/salvodicara/git-recap/main/install.sh | sh
```

**Go** (needs `~/go/bin` on `PATH`):

```sh
go install github.com/salvodicara/git-recap@latest
```

Windows zips are on the [releases page](https://github.com/salvodicara/git-recap/releases/latest).
`git` ≥ 2.37 recommended at runtime.

**Shell completions** — Homebrew installs them automatically and the curl
installer sets up fish; otherwise wire yours in once.

bash (add to `~/.bashrc`):

```sh
source <(git-recap completion bash)
```

zsh:

```sh
git-recap completion zsh > "${fpath[1]}/_git-recap" && compinit
```

fish:

```sh
git-recap completion fish > ~/.config/fish/completions/git-recap.fish
```

## Quickstart

```sh
git recap                        # standup: everything since your last working day
git recap --period week          # this week
git recap --period last-month --write   # save last month into your journal
git recap --format json | jq '.commits | length'
git recap config                 # optional upgrade: workspace roots, profiles
```

## Usage

This is `git-recap help`, verbatim (a test keeps it in sync):

```
git-recap — instant recap of your git work, and a journal that writes itself

Usage:
  git-recap              standup recap: everything since your last working day
  git-recap [flags]      recap any period (prints to stdout; --write also saves)
  git-recap -i           interactive builder: pick profile/period, save a file
  git-recap index        rebuild index.html + per-period pages in the recaps folder
  git-recap config       view or change configuration
  git-recap completion SHELL   print shell completions (bash, zsh, fish)
  git-recap version      print the version (also --version, -v)

Flags:
  --period PERIOD        standup (default), day/today, yesterday,
                         week/this-week, last-week, month/this-month,
                         last-month, quarter, year, last-7-days, last-30-days
  --from / --to          custom range instead, YYYY-MM-DD (--to inclusive)
  --profile NAME         profile to use (default: config's default_profile)
  --org A,B              only these orgs (overrides profile selection)
  --repo X,Y             only these repo names (overrides profile selection)
  --pick                 interactively fuzzy-pick repos for this run
  --fetch                git fetch each repo first (work pushed elsewhere shows up)
  --diffstat             include files changed and +/− lines per commit
  --format F             stdout format: term (default on a terminal), md, json,
                         or html (self-contained report with a heatmap)
  --write                also save the recap as markdown in your recaps folder
  --recaps-folder PATH   save there instead of the configured folder
                         (implies --write; one-off, not saved)
  --frontmatter          add YAML frontmatter to markdown output, so the
                         recaps folder drops into Obsidian as a vault

Zero config: without a config file, git-recap scans the current directory and
counts commits by your git user.email. Run `git-recap config` to set up
workspace roots, profiles, and a recaps folder worth keeping in git.
```

Merge commits are excluded as noise. Piped or in CI, output is plain markdown
and nothing interactive ever triggers.

## Profiles & config

A **profile** picks repos (by org and/or repo name, derived from each repo's
`origin` — GitHub, GitLab, self-hosted all work) and whose commits count, by
author email: exact match, or a whole domain with `@acme.com`.

`git-recap config` is the one command for `~/.config/git-recap/config.toml`:
interactive editor on a terminal, flags for scripts, a plain dump when piped.
Bootstrap from nothing in one line:

```sh
git-recap config --roots ~/Work --profile work --orgs acme --emails me@co.com
```

## The journal

`--write` saves markdown under `<recaps_folder>/<profile>/<year>/<period>.md`
(e.g. `work/2026/2026-06.md`; weeks are `2026-W27.md`, quarters `2026-Q2.md`,
custom ranges `<from>_<to>.md`):

```markdown
# work — 2026-06

## 2026-06-30

### acme/widgets

- `e8dd688` 09:15 — Add retry to upload client
```

The recaps folder becomes its own git repo on first write; `git-recap`
**never commits or pushes** — that's yours. And since every file is
reconstructible from git history, regenerate any period whenever.

**Obsidian:** add `--frontmatter` and the folder is a vault of dated notes
with YAML properties, ready for dataview and graph view.

**A website:** `git recap index` builds `index.html` — per-year contribution
heatmaps and totals per profile — plus an `.html` page next to every journal.
Push the folder to GitHub Pages and your work journal is a site.

## Recipes

The recap is the evidence; pipe it into whatever writes your prose — no API
keys in git-recap, no lock-in:

```sh
# This morning's standup, written for you by your LLM CLI of choice
git recap --format md | claude -p "Write my standup update, 3 bullets max"

# The performance-review brag doc, from a whole year of work
git recap --period year --diffstat --format md | claude -p "Group this year's work into themes"

# Commits this quarter, by repo
git recap --period quarter --format json | jq '.commits | group_by(.repo) | map({repo: .[0].repo, n: length})'

# Your year as a shareable page: heatmap + full journal, one file
git recap --period year --format html > 2026.html
```

## Development

```sh
just check          # build, vet, test, gofmt
just lint           # staticcheck + modernize
just release X.Y.Z  # tag, GitHub release, binaries, brew tap — all local, no CI minutes
```

## License

MIT — see [LICENSE](LICENSE).
