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
- **Any period.** Day, week, month, quarter, year, or a custom `--from`/`--to` range.
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

Files land at `<journal_root>/<profile>/<year>/<filename>.md`, e.g.
`~/git-recap/work/2026/2026-06.md`. Regenerate anytime — output is idempotent.

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
git-recap               # interactive: pick a period and go
git recap --period week # scripted: this week, as a git subcommand
git recap --pick        # fuzzy-pick repos ad hoc
```

`git-recap config` discovers git repos under your workspace root(s), lets you
fuzzy-pick the ones you care about, and writes
`~/.config/git-recap/config.toml` for you. Run it again any time to change
settings — it's the one command for all configuration.

## Usage

```
git-recap [flags]      generate a journal for a period
git-recap config       view or change configuration

  --profile NAME        profile to use (default: config's default_profile)
  --org A,B             only these orgs (overrides profile selection)
  --repo X,Y            only these repo names (overrides profile selection)
  --period PERIOD       day | week | month | quarter | year   (default: month)
  --from YYYY-MM-DD      custom range start (use with --to)
  --to YYYY-MM-DD        custom range end, inclusive (use with --from)
  --pick                interactively fuzzy-pick repos for this run
```

- **Period** sets the default date range and the output filename:
  `2026-06-30.md` (day), `2026-W27.md` (ISO week, Monday start), `2026-06.md`
  (month), `2026-Q2.md` (quarter), `2026.md` (year).
- **`--from`/`--to`** override the range for a custom window (both required).
- **Run bare on a terminal** to pick the period (and profile, if you have more
  than one) interactively. Piped, in CI, or with any flag, `git-recap` runs
  non-interactively using the default profile. Add **`--pick`** to fuzzy-pick
  repos for a single run.

## Profiles & config

A **profile** bundles which repos to include — by GitHub-style org and/or repo
name — and whose commits to count, by author email. Orgs are derived from each
repo's `origin` remote, so many orgs can group into one profile.

`git-recap config` is the single command for everything in
`~/.config/git-recap/config.toml` (which is git-ignored). On a terminal it
opens an interactive editor for **every** setting — workspace roots, journal
root, profiles (add/edit/delete), default profile. You never have to hand-edit
the file.

For scripts and agents, pass flags to set values non-interactively (each flag
replaces that field):

```sh
git-recap config --journal-root ~/journal
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

## Keeping the journal in git

`journal_root` defaults to `~/git-recap` and is initialized as its own git repo
on first write. `git-recap` **never commits or pushes** — that's left entirely
to you:

```sh
cd ~/git-recap
git add . && git commit -m "journal: June 2026"
```

## License

MIT — see [LICENSE](LICENSE).
