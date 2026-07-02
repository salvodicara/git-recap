# git-recap

> What did I do since standup? Last month? All year? Your git history knows —
> `git recap` answers instantly, and can keep a journal you never have to write.

[![CI](https://github.com/salvodicara/git-recap/actions/workflows/ci.yml/badge.svg)](https://github.com/salvodicara/git-recap/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/salvodicara/git-recap)](https://github.com/salvodicara/git-recap/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/salvodicara/git-recap.svg)](https://pkg.go.dev/github.com/salvodicara/git-recap)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

No daemon, no daily capture, no external service, no API keys. Git already
stores every commit with its author and date; `git-recap` reads that — across
**all branches** of **all your repos** — and turns it into a standup answer,
a markdown journal, or JSON for your scripts.

Because the binary is named `git-recap`, git picks it up as a subcommand:
`git recap` works anywhere `git-recap` does.

![git recap in a terminal: one command prints the last working day's commits grouped by day and repo, with a stats summary](demo.gif)

- **Instant, zero config.** `git recap` in any directory shows everything since
  your last working day (on Monday it reaches back to Friday). No setup — it
  scans from where you are and uses your `git config user.email`.
- **Every branch, no double counting.** Commits are collected across all local
  and remote branches, so unmerged or in-review work shows up — and rebased or
  cherry-picked duplicates are folded into one.
- **Any period.** `--period week`, `last-month`, `quarter`, `year`,
  `last-30-days`, or a custom `--from`/`--to`. Backfill a whole year on day one.
- **A journal that writes itself.** `--write` saves the recap as tidy markdown,
  one file per period, in a folder that's its own git repo. Output is
  idempotent — regenerate any period, any time.
- **Human and machine friendly.** Styled terminal output on a TTY (honours
  `NO_COLOR`), plain markdown when piped, `--format json` for scripts and
  agents.

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
| JSON output for scripts & agents         | ✓         | –           |
| Diff stats                               | ✓         | ✓           |
| Fetch first                              | ✓         | ✓           |
| Actively maintained                      | ✓         | last release 2020 |

## Install

`git` (≥ 2.37 recommended) is required at runtime.

**Homebrew** (macOS/Linux):

```sh
brew install salvodicara/tap/git-recap
```

**curl** (prebuilt binary, checksum-verified, no dependencies):

```sh
curl -fsSL https://raw.githubusercontent.com/salvodicara/git-recap/main/install.sh | sh
```

**With Go:**

```sh
go install github.com/salvodicara/git-recap@latest
```

This drops the binary in `go env GOPATH`/bin (usually `~/go/bin`), which must
be on your `PATH`:

```sh
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc   # or ~/.bashrc
exec $SHELL
```

**Windows:** prebuilt zips are attached to
[releases](https://github.com/salvodicara/git-recap/releases/latest).

**From source:** `git clone https://github.com/salvodicara/git-recap && cd git-recap && go install .`

**Shell completions** (flags, period and format values; works for `git recap` too):

```sh
source <(git-recap completion bash)                                    # bash (add to ~/.bashrc)
git-recap completion zsh > "${fpath[1]}/_git-recap" && compinit        # zsh
git-recap completion fish > ~/.config/fish/completions/git-recap.fish # fish
```

## Quickstart

```sh
git recap                      # standup: everything since your last working day
git recap --period week        # this week
git recap --period last-month --write   # save last month into your journal
git recap --format json | jq '.commits | length'
git recap config               # optional: workspace roots, profiles, journal folder
git recap -i                   # interactive builder: pick profile/period, save a file
```

Everything works with zero configuration from your current directory. Config
is an upgrade, not a requirement: it adds workspace roots (recap all your
repos from anywhere), profiles, and a journal folder.

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

- **`standup`** (the default) covers your last working day through now —
  on Monday that includes Friday and the weekend. Configure nothing, just run it.
- **Calendar presets** name the saved file after the window: `2026-06-30.md`
  (day), `2026-W27.md` (ISO week, Monday start), `2026-06.md` (month),
  `2026-Q2.md` (quarter), `2026.md` (year). `this-*`/`last-*` pick the current
  or previous window.
- **Rolling windows** (`last-7-days`, `last-30-days`) cover the last N complete
  days and are named by their span, e.g. `2026-06-24_2026-06-30.md`.
- **`--from`/`--to`** override everything for a custom window (both required,
  `--to` inclusive).
- **All branches.** Commits are collected across every ref, filtered by author
  email, deduplicated across rebases/cherry-picks. Merge commits are excluded
  as noise. Run `--fetch` first if you also commit from another machine.
- **Piped or in CI**, output is plain markdown (or `--format json`); nothing
  interactive ever triggers without a TTY.

## Profiles & config

A **profile** bundles which repos to include — by GitHub-style org and/or repo
name — and whose commits to count, by author email. Emails match exactly
(case-insensitive), or a whole domain when the entry starts with `@`
(`@acme.com` catches every alias at that company). Orgs are derived from each
repo's `origin` remote (host-agnostic: GitHub, GitLab, self-hosted all work).

`git-recap config` is the single command for everything in
`~/.config/git-recap/config.toml`. On a terminal it opens an interactive
editor for every setting — workspace roots, recaps folder, profiles, default
profile. You never have to hand-edit the file.

For scripts and agents, flags set values non-interactively:

```sh
git-recap config --roots ~/Work,~/oss          # workspace roots to scan
git-recap config --recaps-folder ~/Workspace/my-recaps
git-recap config --profile work --orgs acme,acme-labs --emails me@co.com
git-recap config --default-profile work
git-recap config                               # piped/non-TTY: prints current config
```

Bootstrap from nothing in one line:

```sh
git-recap config --roots ~/Work --profile work --orgs acme --emails me@co.com
```

## The journal

`--write` (or the `-i` builder) saves the recap under
`<recaps_folder>/<profile>/<year>/<period>.md`, e.g.
`~/Workspace/my-recaps/work/2026/2026-06.md`:

```markdown
# work — 2026-06

## 2026-06-30

### acme/widgets

- `e8dd688` 09:15 — Add retry to upload client
- `8779a77` 14:32 — Fix null check in parser
```

The recaps folder is initialized as its own git repo on first write;
`git-recap` **never commits or pushes** — that's yours:

```sh
cd ~/Workspace/my-recaps
git add . && git commit -m "recaps: June 2026"
```

Since every file is reconstructible from git history, there's no pressure to
save eagerly — regenerate any period whenever you need it.

### Obsidian

Add `--frontmatter` and every journal gets YAML frontmatter (title, profile,
period, dates, commit count) — point Obsidian at the recaps folder and it's a
vault of dated notes, ready for dataview queries and graph view:

```sh
git recap --period week --write --frontmatter
```

### Your recaps as a website

`git recap index` turns the whole folder into a static site: an `index.html`
with per-year contribution heatmaps and totals for each profile, plus an
`.html` page next to every journal file. It reads the journals themselves, so
it works even for periods whose repos are long gone. Push the folder to GitHub
Pages (or any static host) and your work journal is a website.

## Never write a standup again

The recap is the evidence; your LLM CLI turns it into the prose. No API keys
in git-recap, no lock-in — pipe to whatever you use:

```sh
# This morning's standup update, written for you
git recap --format md | claude -p "Write my standup update from these commits, 3 bullets max"

# The performance-review brag doc, from a whole year of work
git recap --period year --diffstat --format md | claude -p "Group this year's work into themes with impact statements"

# Friday: summarize the week and save the journal
git recap --period week --write --format md | claude -p "Write a short weekly update for my team lead"
```

## Scripts, agents, dashboards

JSON output makes recaps composable:

```sh
# Commits this quarter, by repo
git recap --period quarter --format json | jq '.commits | group_by(.repo) | map({repo: .[0].repo, n: length})'

# Friday cron: save the week's journal
git recap --period week --write

# Your year as a shareable page: heatmap + full journal, one self-contained file
git recap --period year --format html > 2026.html
```

## Development

Common tasks run through [`just`](https://github.com/casey/just):

```sh
just            # list tasks
just check      # build, vet, test, gofmt
just lint       # staticcheck + modernize
just run --period week
```

Releases are cut entirely locally (no CI minutes): tag, GitHub release,
prebuilt binaries via goreleaser, Homebrew tap bump:

```sh
just release 0.2.0
```

GitHub Actions mirror this as a backup: `CI` runs the test gate on every
push/PR (skipping doc-only changes), and `Release` can be dispatched manually
to build binaries if the local goreleaser step was skipped.

## License

MIT — see [LICENSE](LICENSE).
