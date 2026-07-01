package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// preset is a calendar window: a unit (day/week/month/quarter/year) shifted
// back by offset (0 = current, -1 = previous).
type preset struct {
	unit   string
	offset int
}

// presets is the shared period vocabulary for both the CLI (--period) and the
// interactive picker. Calendar-aligned windows only; rolling windows live in
// rollingDays, custom windows use --from/--to.
var presets = map[string]preset{
	"day":        {"day", 0},
	"today":      {"day", 0},
	"yesterday":  {"day", -1},
	"week":       {"week", 0},
	"this-week":  {"week", 0},
	"last-week":  {"week", -1},
	"month":      {"month", 0},
	"this-month": {"month", 0},
	"last-month": {"month", -1},
	"quarter":    {"quarter", 0},
	"year":       {"year", 0},
}

// rollingDays maps a token to a window of the last N *complete* days (ending at
// today 00:00, so it excludes today's in-progress work). Not calendar-aligned.
var rollingDays = map[string]int{
	"last-7-days":  7,
	"last-30-days": 30,
}

// validPeriod reports whether a --period token is a known preset or rolling window.
func validPeriod(p string) bool {
	if _, ok := presets[p]; ok {
		return true
	}
	if _, ok := rollingDays[p]; ok {
		return true
	}
	return p == "standup"
}

// standupRange is [last working day 00:00, tomorrow 00:00): on Monday it
// reaches back to Friday, so weekend work shows and nothing is missed since
// the previous standup. Includes today's commits — they belong in the report.
func standupRange(ref time.Time) (from, to time.Time) {
	y, m, d := ref.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, ref.Location())
	to = midnight.AddDate(0, 0, 1)
	from = midnight.AddDate(0, 0, -1)
	for from.Weekday() == time.Saturday || from.Weekday() == time.Sunday {
		from = from.AddDate(0, 0, -1)
	}
	return
}

// defaultRange returns [from, to) for the current period containing ref.
func defaultRange(period string, ref time.Time) (from, to time.Time) {
	return calendarRange(period, 0, ref)
}

// calendarRange returns [from, to) for a calendar unit shifted back by offset
// units from the one containing ref. Weeks start Monday.
func calendarRange(unit string, offset int, ref time.Time) (from, to time.Time) {
	y, m, d := ref.Date()
	loc := ref.Location()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, loc)
	switch unit {
	case "day":
		from = midnight.AddDate(0, 0, offset)
		to = from.AddDate(0, 0, 1)
	case "week":
		woff := (int(ref.Weekday()) + 6) % 7 // Mon=0 ... Sun=6
		from = midnight.AddDate(0, 0, -woff+7*offset)
		to = from.AddDate(0, 0, 7)
	case "month":
		from = time.Date(y, m, 1, 0, 0, 0, 0, loc).AddDate(0, offset, 0)
		to = from.AddDate(0, 1, 0)
	case "quarter":
		qm := time.Month((int(m)-1)/3*3 + 1)
		from = time.Date(y, qm, 1, 0, 0, 0, 0, loc).AddDate(0, 3*offset, 0)
		to = from.AddDate(0, 3, 0)
	case "year":
		from = time.Date(y, 1, 1, 0, 0, 0, 0, loc).AddDate(offset, 0, 0)
		to = from.AddDate(1, 0, 0)
	}
	return
}

// periodFilename returns the <year> folder and base filename (no extension)
// for a period anchored at t. Weeks use the ISO year/week.
func periodFilename(period string, t time.Time) (year, name string) {
	switch period {
	case "day":
		return t.Format("2006"), t.Format("2006-01-02")
	case "week":
		y, w := t.ISOWeek()
		return fmt.Sprintf("%04d", y), fmt.Sprintf("%04d-W%02d", y, w)
	case "quarter":
		q := (int(t.Month())-1)/3 + 1
		return t.Format("2006"), fmt.Sprintf("%04d-Q%d", t.Year(), q)
	case "year":
		return t.Format("2006"), t.Format("2006")
	default: // month
		return t.Format("2006"), t.Format("2006-01")
	}
}

// rangeName formats an arbitrary [from, to) window as "start_end" using the
// inclusive last day (to is exclusive), e.g. "2026-06-01_2026-06-30".
func rangeName(from, to time.Time) string {
	last := to.AddDate(0, 0, -1)
	return from.Format("2006-01-02") + "_" + last.Format("2006-01-02")
}

// resolveRange picks the date window + output filename from (in priority order):
// an explicit --from/--to window, a calendar preset, or a rolling window. The
// filename always reflects the *actual* resolved range, never the raw token.
func resolveRange(period, fromStr, toStr string, ref time.Time) (from, to time.Time, year, name string, err error) {
	if (fromStr == "") != (toStr == "") {
		err = fmt.Errorf("--from and --to must be used together")
		return
	}
	if fromStr != "" {
		loc := ref.Location()
		if from, err = time.ParseInLocation("2006-01-02", fromStr, loc); err != nil {
			err = fmt.Errorf("invalid --from %q (want YYYY-MM-DD)", fromStr)
			return
		}
		var t time.Time
		if t, err = time.ParseInLocation("2006-01-02", toStr, loc); err != nil {
			err = fmt.Errorf("invalid --to %q (want YYYY-MM-DD)", toStr)
			return
		}
		to = t.AddDate(0, 0, 1) // make --to inclusive of its day
		if !to.After(from) {
			err = fmt.Errorf("--to must not be before --from")
			return
		}
		return from, to, from.Format("2006"), rangeName(from, to), nil
	}
	if period == "standup" {
		from, to = standupRange(ref)
		return from, to, from.Format("2006"), rangeName(from, to), nil
	}
	if p, ok := presets[period]; ok {
		from, to = calendarRange(p.unit, p.offset, ref)
		year, name = periodFilename(p.unit, from)
		return
	}
	if days, ok := rollingDays[period]; ok {
		y, m, d := ref.Date()
		to = time.Date(y, m, d, 0, 0, 0, 0, ref.Location()) // today 00:00, exclusive
		from = to.AddDate(0, 0, -days)
		return from, to, from.Format("2006"), rangeName(from, to), nil
	}
	err = fmt.Errorf("invalid --period %q", period)
	return
}

// writeJournal writes content to recapsFolder/<profile>/<year>/<name>.md and
// ensures recapsFolder is a git repo. It never commits.
func writeJournal(recapsFolder, profile, year, name, content string) (string, error) {
	root := expandTilde(recapsFolder)
	dir := filepath.Join(root, profile, year)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	ensureGitRepo(root)
	return path, nil
}

// ensureGitRepo runs `git init` on root if it isn't already a repo. Best-effort:
// the journal is written regardless. Never stages, commits, or pushes.
func ensureGitRepo(root string) {
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		return
	}
	_ = exec.Command("git", "-C", root, "init").Run()
}
