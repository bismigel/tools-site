package tool

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	// The time zone database is embedded so the answer cannot depend on the
	// host's tzdata (the runtime image is distroless). NFR-6.3, DESIGN §7.
	_ "time/tzdata"
)

const (
	maxCities = 6
	maxDays   = 366
)

type plannerHour struct {
	local   []time.Time
	working []bool
}

// NewMeetingPlanner builds the WorldTimeBuddy-style overlap tool. It is public:
// everything it needs (tzdata) is embedded, so there is no egress.
func NewMeetingPlanner() Tool {
	return Tool{
		Name:        "meeting_planner",
		Summary:     "Find a meeting slot across time zones",
		Description: "Shows a full day side by side for two to six cities and names the longest window in which everyone is inside their own working hours.",
		Visibility:  Public,
		Inputs: []Input{
			{Name: "cities", Label: "Cities", Kind: Text, Required: true, Min: 3, Max: 200,
				Default: "London, New York",
				Help:    "2–6 cities, comma separated: London, New York, Singapore, Sydney."},
			{Name: "date", Label: "Date", Kind: Text,
				Help: "YYYY-MM-DD. Blank means today."},
			{Name: "work_start", Label: "Working hours from", Kind: Text, Default: "09:00",
				Help: "In each city's own local time."},
			{Name: "work_end", Label: "Working hours to", Kind: Text, Default: "17:00"},
			{Name: "length", Label: "Meeting length", Kind: Select,
				Options: []string{"30", "45", "60"}, Default: "60", Help: "Minutes."},
		},
		Run: runMeetingPlanner,
	}
}

func runMeetingPlanner(_ context.Context, in Values) (Result, error) {
	now := time.Now()

	cities, err := parseCities(in.Get("cities"))
	if err != nil {
		return Result{}, err
	}

	anchorLoc, err := time.LoadLocation(cities[0].Zone)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %s", ErrBadInput, err)
	}

	date, err := parseDate(in.Get("date"), anchorLoc, now)
	if err != nil {
		return Result{}, err
	}

	start, end, err := parseWindow(in.Get("work_start"), in.Get("work_end"))
	if err != nil {
		return Result{}, err
	}

	length, err := strconv.Atoi(in.Get("length"))
	if err != nil || length < 15 || length > 480 {
		return Result{}, fmt.Errorf("%w: meeting length must be 30, 45 or 60 minutes", ErrBadInput)
	}

	// One row per hour of the anchor's local day; each cell is that instant in
	// the city's own zone, and "working" uses the city's own clock.
	rows := make([]plannerHour, 24)
	for h := 0; h < 24; h++ {
		at := time.Date(date.Year(), date.Month(), date.Day(), h, 0, 0, 0, anchorLoc)
		row := plannerHour{local: make([]time.Time, len(cities)), working: make([]bool, len(cities))}
		for i, c := range cities {
			loc, err := time.LoadLocation(c.Zone)
			if err != nil {
				return Result{}, fmt.Errorf("%w: %s", ErrBadInput, err)
			}
			t := at.In(loc)
			row.local[i] = t
			row.working[i] = inWindow(t, start, end)
		}
		rows[h] = row
	}

	needed := (length + 59) / 60
	bestStart, bestLen := longestOverlap(rows)
	found := bestLen >= needed

	var warnings []string
	for _, c := range cities {
		if changed, detail := offsetChange(c, date, anchorLoc); changed {
			warnings = append(warnings, fmt.Sprintf("%s: %s", c.Name, detail))
		}
	}
	if !found {
		warnings = append(warnings, fmt.Sprintf(
			"No window of %d minutes with everyone inside %s–%s. Try a shorter meeting, another date, or wider working hours.",
			length, fmtWindowTime(start), fmtWindowTime(end)))
	}

	table := &Table{
		Caption: fmt.Sprintf("%s in %s (anchor) · working hours %s in each city's own time",
			date.Format("Mon 2 Jan 2006"), cities[0].Name, fmtWindow(start, end)),
		Columns: append([]string{"Local"}, cityNames(cities)...),
	}
	for h := 0; h < 24; h++ {
		cells := []Cell{{Text: fmt.Sprintf("%02d:00", h), Tone: ToneMuted}}
		for i, t := range rows[h].local {
			cell := Cell{Text: t.Format("15:04"), Tone: ToneMuted}
			if rows[h].working[i] {
				cell.Tone = ToneNormal
			}
			if found && h >= bestStart && h < bestStart+bestLen {
				cell.Tone = ToneEmphasis
			}
			cell.Note = dayNote(t, date)
			cells = append(cells, cell)
		}
		table.Rows = append(table.Rows, cells)
	}

	jsonHours := make([]map[string]any, 0, 24)
	for h := 0; h < 24; h++ {
		times := make([]string, len(cities))
		for i, t := range rows[h].local {
			times[i] = t.Format("15:04")
		}
		jsonHours = append(jsonHours, map[string]any{
			"anchor":  fmt.Sprintf("%02d:00", h),
			"times":   times,
			"working": rows[h].working,
		})
	}

	return Result{
		Title:    verdict(rows, cities, bestStart, bestLen, needed, length),
		Table:    table,
		Warnings: warnings,
		JSON: map[string]any{
			"date":   date.Format("2006-01-02"),
			"anchor": map[string]string{"name": cities[0].Name, "zone": cities[0].Zone},
			"cities": cityJSON(cities),
			"window": map[string]string{"start": fmtWindowTime(start), "end": fmtWindowTime(end)},
			"length": length,
			"overlap": map[string]any{
				"found":   found,
				"start":   fmt.Sprintf("%02d:00", bestStart),
				"hours":   bestLen,
				"minutes": bestLen * 60,
			},
			"hours": jsonHours,
		},
	}, nil
}

func parseCities(raw string) ([]city, error) {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '\n'
	})
	seen := make(map[string]bool, len(parts))
	out := make([]city, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		c, err := resolveCity(p)
		if err != nil {
			return nil, err
		}
		if seen[c.Zone] {
			return nil, fmt.Errorf("%w: %s is listed twice", ErrBadInput, c.Name)
		}
		seen[c.Zone] = true
		out = append(out, c)
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("%w: give at least two cities", ErrBadInput)
	}
	if len(out) > maxCities {
		return nil, fmt.Errorf("%w: at most %d cities (you gave %d)", ErrBadInput, maxCities, len(out))
	}
	return out, nil
}

func parseDate(raw string, loc *time.Location, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	today := now.In(loc)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)
	if raw == "" {
		return today, nil
	}
	d, err := time.ParseInLocation("2006-01-02", raw, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: date must be YYYY-MM-DD", ErrBadInput)
	}
	days := d.Sub(today).Hours() / 24
	if days < -maxDays || days > maxDays {
		return time.Time{}, fmt.Errorf("%w: date must be within %d days of today", ErrBadInput, maxDays)
	}
	return d, nil
}

func parseWindow(startRaw, endRaw string) (time.Duration, time.Duration, error) {
	start, err1 := time.Parse("15:04", strings.TrimSpace(startRaw))
	end, err2 := time.Parse("15:04", strings.TrimSpace(endRaw))
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("%w: times must be HH:MM", ErrBadInput)
	}
	s := time.Duration(start.Hour())*time.Hour + time.Duration(start.Minute())*time.Minute
	e := time.Duration(end.Hour())*time.Hour + time.Duration(end.Minute())*time.Minute
	if e <= s {
		return 0, 0, fmt.Errorf("%w: the working window must end after it starts", ErrBadInput)
	}
	return s, e, nil
}

func inWindow(t time.Time, start, end time.Duration) bool {
	day := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute
	return day >= start && day < end
}

// longestOverlap returns the first longest run of hours where every city is
// inside its own working window.
func longestOverlap(rows []plannerHour) (int, int) {
	bestStart, bestLen := 0, 0
	for i := 0; i < len(rows); i++ {
		if !allWorking(rows[i].working) {
			continue
		}
		j := i
		for j < len(rows) && allWorking(rows[j].working) {
			j++
		}
		if j-i > bestLen {
			bestStart, bestLen = i, j-i
		}
		i = j
	}
	return bestStart, bestLen
}

func allWorking(w []bool) bool {
	for _, v := range w {
		if !v {
			return false
		}
	}
	return true
}

func verdict(rows []plannerHour, cities []city, start, bestLen, needed, meeting int) string {
	if bestLen < needed {
		bestHour, bestCount := 0, -1
		for h := range rows {
			count := 0
			for _, w := range rows[h].working {
				count += btoi(w)
			}
			if count > bestCount {
				bestHour, bestCount = h, count
			}
		}
		return fmt.Sprintf("No %d-minute overlap; best hour is %02d:00 %s (%d of %d cities in hours)",
			meeting, bestHour, cities[0].Name, bestCount, len(cities))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d h overlap: %02d:00–%02d:00 %s", bestLen, start, start+bestLen, cities[0].Name)
	for i := 1; i < len(cities); i++ {
		from := rows[start].local[i].Format("15:04")
		to := rows[start+bestLen-1].local[i].Add(time.Hour).Format("15:04")
		fmt.Fprintf(&b, " · %s %s–%s", cities[i].Name, from, to)
	}
	return b.String()
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func dayNote(t time.Time, anchor time.Time) string {
	ty, tm, td := t.Date()
	ay, am, ad := anchor.Date()
	switch {
	case ty > ay || (ty == ay && (tm > am || (tm == am && td > ad))):
		return "+1 day"
	case ty < ay || (ty == ay && (tm < am || (tm == am && td < ad))):
		return "−1 day"
	default:
		return ""
	}
}

// offsetChange reports a summer-time transition on the chosen date, so the grid
// is never silently an hour out. DST itself is the tz database's job.
func offsetChange(c city, date time.Time, anchorLoc *time.Location) (bool, string) {
	loc, err := time.LoadLocation(c.Zone)
	if err != nil {
		return false, ""
	}
	begin := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, anchorLoc).In(loc)
	end := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, anchorLoc).In(loc)
	_, offBegin := begin.Zone()
	_, offEnd := end.Zone()
	if offBegin == offEnd {
		return false, ""
	}
	beginName, _ := begin.Zone()
	endName, _ := end.Zone()
	return true, fmt.Sprintf("clocks change on this date (%s → %s); times after the change are shifted", beginName, endName)
}

func cityNames(c []city) []string {
	out := make([]string, len(c))
	for i := range c {
		out[i] = c[i].Name
	}
	return out
}

func cityJSON(c []city) []map[string]string {
	out := make([]map[string]string, len(c))
	for i := range c {
		out[i] = map[string]string{"name": c[i].Name, "zone": c[i].Zone}
	}
	return out
}

func fmtWindow(start, end time.Duration) string {
	return fmtWindowTime(start) + "–" + fmtWindowTime(end)
}

func fmtWindowTime(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}
