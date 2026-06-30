# git-recap

Reconstruct a work journal from your **git history** — no daemon, no daily
capture, no external service. Git already stores every commit with its author
and date, so `git-recap` just reads that and writes a tidy markdown journal for
any past day, week, month, quarter, or year. Backfill a whole year on day one.

Because the binary is named `git-recap`, git also picks it up as a subcommand:
`git recap` works anywhere `git-recap` does.

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

This drops the `git-recap` binary in your `~/go/bin`; make sure that's on your
`PATH`.

**From source:**

```sh
git clone https://github.com/salvodicara/git-recap
cd git-recap
./install.sh            # builds and installs to ~/.local/bin (override with PREFIX=)
```

## Quickstart

```sh
git-recap init          # scan your repos, pick some, name a profile
git-recap               # journal for the current month (default profile)
git recap --period week # same thing as a git subcommand
git recap --pick        # fuzzy-pick repos ad hoc
```

`init` discovers git repos under your workspace root(s), lets you fuzzy-pick the
ones you care about, and writes `~/.config/git-recap/config.toml` for you. You
never have to hand-edit the config (though you can).

## Usage

```
git-recap [flags]      generate a journal for a period
git-recap init         first-run setup

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
- With **no flags**, the default profile is used non-interactively. Add
  **`--pick`** to fuzzy-pick repos ad hoc instead.

## Profiles & config

A **profile** bundles which repos to include — by GitHub-style org and/or repo
name — and whose commits to count, by author email. Orgs are derived from each
repo's `origin` remote, so many orgs can group into one profile.

`git-recap init` writes your config to `~/.config/git-recap/config.toml` (it's
git-ignored). You never have to hand-edit it; re-run `init` to change profiles.

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
