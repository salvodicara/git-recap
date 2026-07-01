# git-recap

> Reconstruct a work journal from your **git history** — no daemon, no daily capture, no external service.

[![Go Reference](https://pkg.go.dev/badge/github.com/salvodicara/git-recap.svg)](https://pkg.go.dev/github.com/salvodicara/git-recap)
[![Go Report Card](https://goreportcard.com/badge/github.com/salvodicara/git-recap)](https://goreportcard.com/report/github.com/salvodicara/git-recap)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Git already stores every commit with its author and date, so `git-recap` just
reads that and writes a tidy markdown journal for any past day, week, month,
quarter, or year. Backfill a whole year on day one.

Because the binary is named `git-recap`, git also picks it up as a subcommand:
`git recap` works anywhere `git-recap` does.

- **Zero setup, zero services.** Reads local git history — nothing to host, no API keys, no tracking.
- **Any period.** Presets like `today`, `last-week`, `last-month`, `last-7-days`, the current day/week/month/quarter/year, or a custom `--from`/`--to` range.
- **Every branch.** Counts your commits across *all* branches (local and remote), so unmerged or in-review work still shows up — not just what's on the current branch.
- **Profiles.** Group repos by org or name and count only your commits, by author email.
- **Human *and* script friendly.** An interactive TUI in your terminal; full flags for agents and CI.

## What it produces

One markdown file per run, days as sections, grouped by repo:

```markdown
# work — 2026-06

## 2026-06-30

### acme/widgets

- `e8dd688` 09:15 — Add retry to upload client
- `8779a77` 14:32 — Fix null check in parser
```

Files land at `<recaps_folder>/<profile>/<year>/<filename>.md`, e.g.
`~/Workspace/my-recaps/work/2026/2026-06.md`. Regenerate anytime — output is idempotent.

## Install

`git` is required at runtime.

**With Go** (recommended — one command):

```sh
go install github.com/salvodicara/git-recap@latest
```

This drops the `git-recap` binary in your Go bin directory (`go env GOPATH`/bin,
usually `~/go/bin`). That must be on your `PATH` — if `git-recap` isn't found
after install, add it:

```sh
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc   # or ~/.bashrc
exec $SHELL
```

**From source:**

```sh
git clone https://github.com/salvodicara/git-recap
cd git-recap
./install.sh            # builds and installs to ~/.local/bin (override with PREFIX=)
```

## Quickstart

```sh
git-recap config        # first run: scan repos, pick some, name a profile
git-recap               # interactive: pick a period (or custom range) and go
git recap --period week        # scripted: this week, as a git subcommand
git recap --period last-month  # the month that just ended
git recap --pick               # fuzzy-pick repos ad hoc
```

`git-recap config` discovers git repos under your workspace root(s), lets you
fuzzy-pick the ones you care about, and writes
`~/.config/git-recap/config.toml` for you. Run it again any time to change
settings — it's the one command for all configuration.

## Usage

```
git-recap [flags]      generate a journal for a period
git-recap config       view or change configuration
git-recap version      print the version (also --version, -v)

  --profile NAME        profile to use (default: config's default_profile)
  --org A,B             only these orgs (overrides profile selection)
  --repo X,Y            only these repo names (overrides profile selection)
  --period PERIOD       a period preset (default: month):
                          day/today, yesterday,
                          week/this-week, last-week,
                          month/this-month, last-month,
                          quarter, year, last-7-days, last-30-days
  --from YYYY-MM-DD      custom range start (use with --to)
  --to YYYY-MM-DD        custom range end, inclusive (use with --from)
  --pick                interactively fuzzy-pick repos for this run
```

- **Period** sets the date range and output filename. Calendar presets name the
  file after the window: `2026-06-30.md` (day), `2026-W27.md` (ISO week, Monday
  start), `2026-06.md` (month), `2026-Q2.md` (quarter), `2026.md` (year).
  The `this-*`/`last-*` variants select the current or previous window;
  `today`/`yesterday` are day aliases.
- **Rolling windows** (`last-7-days`, `last-30-days`) cover the last N *complete*
  days and are named by their span, e.g. `2026-06-24_2026-06-30.md`.
- **`--from`/`--to`** override everything for a custom window (both required,
  `--to` inclusive); the file is named `<from>_<to>.md`, e.g.
  `2026-05-03_2026-05-19.md`.
- **All branches.** Commits are collected across every branch (local and
  remote-tracking), filtered to your author email(s), so work you never merged or
  checked back out is still captured. Merge commits are excluded as noise.
- **Run bare on a terminal** to pick the period — or a custom range — (and
  profile, if you have more than one) interactively. Piped, in CI, or with any
  flag, `git-recap` runs non-interactively using the default profile. Add
  **`--pick`** to fuzzy-pick repos for a single run.

## Profiles & config

A **profile** bundles which repos to include — by GitHub-style org and/or repo
name — and whose commits to count, by author email. Orgs are derived from each
repo's `origin` remote, so many orgs can group into one profile.

`git-recap config` is the single command for everything in
`~/.config/git-recap/config.toml` (which is git-ignored). On a terminal it
opens an interactive editor for **every** setting — workspace roots, recaps
folder, profiles (add/edit/delete), default profile. You never have to hand-edit
the file.

For scripts and agents, pass flags to set values non-interactively (each flag
replaces that field):

```sh
git-recap config --recaps-folder ~/Workspace/my-recaps
git-recap config --roots ~/Work,~/oss          # workspace roots to scan
git-recap config --default-profile personal
git-recap config --profile work --orgs acme,acme-labs --emails me@co.com
git-recap config --delete-profile personal
git-recap config                               # piped/non-TTY: prints current config
```

`--orgs`/`--repos`/`--emails` apply to the profile named by `--profile`; a new
profile is created if it doesn't exist. You can bootstrap from nothing in one
line:

```sh
git-recap config --roots ~/Work --profile work --orgs acme --emails me@co.com
```

## Keeping the recaps in git

The **recaps folder** defaults to `<first workspace root>/my-recaps` (e.g.
`~/Workspace/my-recaps`) and is initialized as its own git repo on first write.
`git-recap` **never commits or pushes** — that's left entirely to you:

```sh
cd ~/Workspace/my-recaps
git add . && git commit -m "recaps: June 2026"
```

## Development

Common tasks run through [`just`](https://github.com/casey/just):

```sh
just            # list tasks
just check      # build, vet, test, gofmt
just lint       # staticcheck + modernize
just run --period week
```

Releases are cut locally (no CI minutes needed) — this tags, publishes a GitHub
release, and updates the Homebrew tap at `../homebrew-tap`:

```sh
just release 0.2.0
```

GitHub Actions mirror this as a backup: `CI` runs the test gate on every push/PR,
and `Release` builds prebuilt binaries when you push a `v*` tag.

## License

MIT — see [LICENSE](LICENSE).
