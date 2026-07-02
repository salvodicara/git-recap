package main

import (
	"html/template"
	"maps"
	"slices"
	"strings"
	"time"
)

// renderHTML produces a self-contained report: contribution heatmap + the
// journal listing. No external assets, no JS — <details> gives collapsing,
// CSS handles dark mode. Palette: sequential blue ramp (light→dark, own steps
// per mode), neutral zero cells, ink tokens for text; low-step contrast is
// relieved by per-cell tooltips and the listing below (the "table view").
func renderHTML(r Recap) string {
	counts := map[string]int{}
	for _, c := range r.Commits {
		counts[c.When.Format("2006-01-02")]++
	}
	maxN := 0
	for _, n := range counts {
		maxN = max(maxN, n)
	}

	type cell struct {
		Date    string
		Count   int
		Level   int // 0 = none, 1..4 = quartile of the period's max
		InRange bool
	}
	// Monday-aligned week columns covering [From, To).
	start := r.From
	for start.Weekday() != time.Monday {
		start = start.AddDate(0, 0, -1)
	}
	var weeks [][]cell
	for d := start; d.Before(r.To); {
		var wk []cell
		for range 7 {
			key := d.Format("2006-01-02")
			c := cell{Date: key, Count: counts[key], InRange: !d.Before(r.From) && d.Before(r.To)}
			if c.Count > 0 {
				c.Level = (c.Count*4 + maxN - 1) / maxN // ceil(4n/max) → 1..4
			}
			wk = append(wk, c)
			d = d.AddDate(0, 0, 1)
		}
		weeks = append(weeks, wk)
	}

	type commitRow struct{ Hash, Time, Subject, Stat string }
	type repoGroup struct {
		Slug    string
		Commits []commitRow
	}
	type dayGroup struct {
		Date, Weekday string
		Count         int
		Repos         []repoGroup
	}
	var days []dayGroup
	byDay := groupByDay(r.Commits)
	for _, day := range slices.Sorted(maps.Keys(byDay)) {
		t, _ := time.Parse("2006-01-02", day)
		dg := dayGroup{Date: day, Weekday: t.Format("Monday"), Count: len(byDay[day])}
		repos := byRepo(byDay[day])
		for _, slug := range slices.Sorted(maps.Keys(repos)) {
			rg := repoGroup{Slug: slug}
			for _, c := range repos[slug] {
				rg.Commits = append(rg.Commits, commitRow{
					Hash: shortHash(c.Hash), Time: c.When.Format("15:04"),
					Subject: c.Subject, Stat: strings.TrimPrefix(statSuffix(c), " "),
				})
			}
			dg.Repos = append(dg.Repos, rg)
		}
		days = append(days, dg)
	}

	data := struct {
		Title, Summary string
		Weeks          [][]cell
		Days           []dayGroup
	}{r.Profile + " — " + r.Name, r.Stats().summary(), weeks, days}

	var b strings.Builder
	// The template is static and tested; an error here is a programming bug.
	if err := htmlTmpl.Execute(&b, data); err != nil {
		panic(err)
	}
	return b.String()
}

var htmlTmpl = template.Must(template.New("recap").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root {
  --surface: #fcfcfb; --page: #f9f9f7;
  --ink: #0b0b0b; --ink-2: #52514e; --ink-3: #898781;
  --hairline: #e1e0d9; --ring: rgba(11,11,11,0.10);
  --lvl0: #efeeea; --lvl1: #b7d3f6; --lvl2: #6da7ec; --lvl3: #2a78d6; --lvl4: #184f95;
}
@media (prefers-color-scheme: dark) {
  :root {
    --surface: #1a1a19; --page: #0d0d0d;
    --ink: #ffffff; --ink-2: #c3c2b7; --ink-3: #898781;
    --hairline: #2c2c2a; --ring: rgba(255,255,255,0.10);
    --lvl0: #262624; --lvl1: #184f95; --lvl2: #256abf; --lvl3: #3987e5; --lvl4: #86b6ef;
  }
}
body { margin: 0; background: var(--page); color: var(--ink);
  font: 15px/1.5 system-ui, -apple-system, "Segoe UI", sans-serif; }
main { max-width: 860px; margin: 0 auto; padding: 32px 20px 64px; }
h1 { font-size: 22px; margin: 0 0 4px; }
.summary { color: var(--ink-2); margin: 0 0 24px; }
.card { background: var(--surface); border: 1px solid var(--ring);
  border-radius: 8px; padding: 16px; margin-bottom: 24px; overflow-x: auto; }
.heatmap { display: flex; gap: 8px; align-items: flex-start; }
.wdays { display: grid; grid-template-rows: repeat(7, 12px); gap: 2px;
  font-size: 10px; color: var(--ink-3); }
.wdays span:nth-child(even) { visibility: hidden; }
.grid { display: grid; grid-auto-flow: column; grid-template-rows: repeat(7, 12px);
  grid-auto-columns: 12px; gap: 2px; }
.grid i { border-radius: 3px; background: var(--lvl0); }
.grid i.out { visibility: hidden; }
.grid i.l1 { background: var(--lvl1); } .grid i.l2 { background: var(--lvl2); }
.grid i.l3 { background: var(--lvl3); } .grid i.l4 { background: var(--lvl4); }
.legend { display: flex; gap: 3px; align-items: center; margin-top: 10px;
  font-size: 11px; color: var(--ink-3); }
.legend i { width: 12px; height: 12px; border-radius: 3px; display: inline-block; }
details { border-bottom: 1px solid var(--hairline); padding: 6px 0; }
summary { cursor: pointer; font-weight: 600; }
summary .n { color: var(--ink-3); font-weight: 400; }
h3 { font-size: 13px; margin: 10px 0 4px; color: var(--ink-2); }
ul { list-style: none; margin: 0; padding: 0 0 6px; }
li { padding: 2px 0; }
.meta { color: var(--ink-3); font-family: ui-monospace, monospace; font-size: 12px;
  font-variant-numeric: tabular-nums; margin-right: 6px; }
.stat { color: var(--ink-3); font-size: 12px; margin-left: 6px; }
.empty { color: var(--ink-2); }
</style>
</head>
<body>
<main>
<h1>{{.Title}}</h1>
<p class="summary">{{.Summary}}</p>
<div class="card">
  <div class="heatmap">
    <div class="wdays"><span>Mon</span><span>Tue</span><span>Wed</span><span>Thu</span><span>Fri</span><span>Sat</span><span>Sun</span></div>
    <div class="grid">
    {{- range .Weeks}}{{range .}}
      <i class="{{if not .InRange}}out{{else if .Level}}l{{.Level}}{{end}}" title="{{.Date}} · {{.Count}} commits"></i>
    {{- end}}{{end}}
    </div>
  </div>
  <div class="legend">Less
    <i style="background:var(--lvl0)"></i><i style="background:var(--lvl1)"></i><i style="background:var(--lvl2)"></i><i style="background:var(--lvl3)"></i><i style="background:var(--lvl4)"></i>
  More</div>
</div>
{{if not .Days}}<p class="empty">No commits in this period.</p>{{end}}
{{- range .Days}}
<details open>
  <summary>{{.Date}} · {{.Weekday}} <span class="n">({{.Count}})</span></summary>
  {{- range .Repos}}
  <h3>{{.Slug}}</h3>
  <ul>
    {{- range .Commits}}
    <li><span class="meta">{{.Hash}} {{.Time}}</span>{{.Subject}}{{if .Stat}}<span class="stat">{{.Stat}}</span>{{end}}</li>
    {{- end}}
  </ul>
  {{- end}}
</details>
{{- end}}
</main>
</body>
</html>
`))
