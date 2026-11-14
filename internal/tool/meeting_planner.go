package tool

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	// The time zone database is embedded so the answer cannot depend on the
	// host's tzdata (the runtime image is distroless). NFR-6.3, DESIGN §7.
	_ "time/tzdata"
)

// nowFunc is a seam for tests; production uses the wall clock.
var nowFunc = time.Now

const (
	maxCities  = 6
	maxDays    = 366
	maxSlots   = 3
	maxSuggest = 40
)

// suggestionNames is the datalist for the city fields. Small on purpose: the
// resolver accepts any IANA zone and ~80 aliases, this is just the fast path.
var suggestionNames = []string{
	"London", "New York", "San Francisco", "Los Angeles", "Chicago", "Toronto",
	"Vancouver", "Mexico City", "São Paulo", "Buenos Aires", "Dublin", "Lisbon",
	"Madrid", "Paris", "Berlin", "Amsterdam", "Zurich", "Stockholm", "Warsaw",
	"Copenhagen", "Istanbul", "Dubai", "Bangalore", "Mumbai", "Singapore",
	"Hong Kong", "Shanghai", "Tokyo", "Seoul", "Sydney", "Melbourne", "Auckland",
}

type plannerHour struct {
	local   []time.Time
	working []bool
}

type slot struct {
	start, hours int
}

// NewMeetingPlanner builds the WorldTimeBuddy-style overlap tool: one row per
// hour, one column per city, working hours shaded, best windows named.
func NewMeetingPlanner() Tool {
	suggestions := suggestionNames
	return Tool{
		Name:        "meeting_planner",
		Summary:     "Find a meeting slot across time zones",
		Description: "Shows a full day side by side for two to six cities, shades each city's own working hours, and names up to three windows that work for everyone.",
		Visibility:  Public,
		Submit:      SubmitGet,
		// City 1 is the anchor, so that is the field worth guessing.
		DetectTimezone: "city1",
		Inputs: []Input{
			{Name: "city1", Group: "Cities", Label: "City 1 (anchor)", Kind: Text,
				Default: "Dubai", Suggestions: suggestions,
				Help: "The rows and the working window are shown in this city's time."},
			{Name: "city2", Group: "Cities", Label: "City 2", Kind: Text,
				Default: "Manila", Suggestions: suggestions},
			{Name: "city3", Group: "Cities", Label: "City 3", Kind: Text, Suggestions: suggestions},
			{Name: "city4", Group: "Cities", Label: "City 4", Kind: Text, Suggestions: suggestions},
			{Name: "city5", Group: "Cities", Label: "City 5", Kind: Text, Suggestions: suggestions},
			{Name: "city6", Group: "Cities", Label: "City 6", Kind: Text, Suggestions: suggestions},
			{Name: "date", Group: "Working window", Label: "Date", Kind: Text, Help: "YYYY-MM-DD. Blank means today."},
			{Name: "work_start", Group: "Working window", Label: "Working hours from", Kind: Text, Default: "09:00",
				Help: "In each city's own local time."},
			{Name: "work_end", Group: "Working window", Label: "Working hours to", Kind: Text, Default: "17:00"},
			{Name: "length", Group: "Working window", Label: "Meeting length", Kind: Select,
				Options: []string{"30", "45", "60"}, Default: "60", Help: "Minutes."},
			{Name: "format", Group: "Working window", Label: "Clock", Kind: Select,
				Options: []string{"12", "24"}, Default: "12", Help: "12-hour (AM/PM) or 24-hour."},
		},
		Run: runMeetingPlanner,
	}
}

func runMeetingPlanner(_ context.Context, in Values) (Result, error) {
	now := nowFunc()

	cities, err := citiesFromInput(in)
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
	use12 := in.Get("format") == "12"

	// Load each zone once: LoadLocation is not cheap, and the 24-row loop below
	// was paying for it per city per hour (782 µs per run before this).
	locs := make([]*time.Location, len(cities))
	for i, c := range cities {
		loc, err := time.LoadLocation(c.Zone)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %s", ErrBadInput, err)
		}
		locs[i] = loc
	}

	// One row per hour of the anchor's local day; each cell is that instant in
	// the city's own zone, and "working" uses the city's own clock.
	rows := make([]plannerHour, 24)
	for h := 0; h < 24; h++ {
		at := time.Date(date.Year(), date.Month(), date.Day(), h, 0, 0, 0, anchorLoc)
		row := plannerHour{local: make([]time.Time, len(cities)), working: make([]bool, len(cities))}
		for i := range cities {
			t := at.In(locs[i])
			row.local[i] = t
			row.working[i] = inWindow(t, start, end)
		}
		rows[h] = row
	}

	slots := qualifyingSlots(rows, length)
	found := len(slots) > 0

	nowLocal := now.In(anchorLoc)
	isToday := nowLocal.Year() == date.Year() && nowLocal.YearDay() == date.YearDay()
	nowParts := make([]string, 0, len(cities))
	for i := range cities {
		nowParts = append(nowParts, clockMinute(now.In(locs[i]), use12)+" "+cities[i].Name)
	}

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
		Caption: fmt.Sprintf("%s · %s local in each city",
			date.Format("Mon 2 Jan 2006"), fmtWindow(start, end)),
		Columns: append([]string{"Local"}, cityNames(cities)...),
	}
	best := slot{}
	if found {
		best = slots[0]
	}
	for h := 0; h < 24; h++ {
		cells := []Cell{{Text: clock(h*60, use12), Tone: ToneMuted}}
		if isToday && h == nowLocal.Hour() {
			cells[0].Note = "now"
			cells[0].Now = true
		}
		for i, t := range rows[h].local {
			cell := Cell{Text: clockMinute(t, use12), Tone: ToneMuted}
			if rows[h].working[i] {
				cell.Tone = ToneActive
			}
			if found && h >= best.start && h < best.start+best.hours {
				cell.Tone = ToneEmphasis
			}
			cell.Note = dayNote(t, date)
			cells = append(cells, cell)
		}
		table.Rows = append(table.Rows, cells)
	}

	// The best slot is the title; only alternatives are listed below it.
	// "Now" first: it answers the question the grid cannot.
	lines := []Line{{Kind: LineKeyValue, Key: "Now", Value: strings.Join(nowParts, " · ")}}
	for i := 1; i < len(slots); i++ {
		s := slots[i]
		key := fmt.Sprintf("Alternative %d · %s", i, humanHours(s.hours))
		lines = append(lines, Line{Kind: LineKeyValue, Key: key, Value: slotSummary(rows, cities, s, use12, 0)})
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
	jsonSlots := make([]map[string]any, 0, len(slots))
	for _, s := range slots {
		jsonSlots = append(jsonSlots, map[string]any{
			"anchor_start": fmt.Sprintf("%02d:00", s.start),
			"hours":        s.hours,
			"minutes":      s.hours * 60,
		})
	}

	return Result{
		Title:    titleFor(rows, cities, slots, length, use12),
		Lines:    lines,
		Table:    table,
		Warnings: warnings,
		JSON: map[string]any{
			"date":   date.Format("2006-01-02"),
			"anchor": map[string]string{"name": cities[0].Name, "zone": cities[0].Zone},
			"cities": cityJSON(cities),
			"window": map[string]string{"start": fmtWindowTime(start), "end": fmtWindowTime(end)},
			"length": length,
			"slots":  jsonSlots,
			"now": map[string]any{
				"is_today": isToday,
				"anchor":   fmt.Sprintf("%02d:%02d", nowLocal.Hour(), nowLocal.Minute()),
			},
			"hours": jsonHours,
		},
	}, nil
}

// qualifyingSlots returns up to three non-overlapping runs that are long enough,
// longest first. Runs are disjoint by construction.
func qualifyingSlots(rows []plannerHour, meeting int) []slot {
	var all []slot
	for i := 0; i < len(rows); i++ {
		if !allWorking(rows[i].working) {
			continue
		}
		j := i
		for j < len(rows) && allWorking(rows[j].working) {
			j++
		}
		if (j-i)*60 >= meeting {
			all = append(all, slot{start: i, hours: j - i})
		}
		i = j
	}
	sort.SliceStable(all, func(a, b int) bool {
		if all[a].hours != all[b].hours {
			return all[a].hours > all[b].hours
		}
		return all[a].start < all[b].start
	})
	if len(all) > maxSlots {
		all = all[:maxSlots]
	}
	sort.SliceStable(all, func(a, b int) bool { return all[a].start < all[b].start })
	return all
}

func titleFor(rows []plannerHour, cities []city, slots []slot, meeting int, use12 bool) string {
	if len(slots) == 0 {
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
		return fmt.Sprintf("No %d-minute overlap; the closest hour is %s %s (%d of %d cities in hours)",
			meeting, clock(bestHour*60, use12), cities[0].Name, bestCount, len(cities))
	}
	s := slots[0]
	return fmt.Sprintf("%s overlap · %s", humanHours(s.hours), slotSummary(rows, cities, s, use12, 0))
}

// slotSummary renders one slot in every city, e.g.
// "14:00–17:00 London · 09:00–12:00 New York".
func slotSummary(rows []plannerHour, cities []city, s slot, use12 bool, from int) string {
	parts := make([]string, 0, len(cities))
	for i := from; i < len(cities); i++ {
		frm := rows[s.start].local[i]
		to := rows[s.start+s.hours-1].local[i].Add(time.Hour)
		parts = append(parts, fmt.Sprintf("%s–%s %s", clockMinute(frm, use12), clockMinute(to, use12), cities[i].Name))
	}
	return strings.Join(parts, " · ")
}

func citiesFromInput(in Values) ([]city, error) {
	seen := make(map[string]bool, maxCities)
	out := make([]city, 0, maxCities)
	for i := 1; i <= maxCities; i++ {
		raw := strings.TrimSpace(in.Get(fmt.Sprintf("city%d", i)))
		if raw == "" {
			continue
		}
		c, err := resolveCity(raw)
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

func allWorking(w []bool) bool {
	for _, v := range w {
		if !v {
			return false
		}
	}
	return true
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

func humanHours(h int) string {
	if h == 1 {
		return "1 hour"
	}
	return fmt.Sprintf("%d hours", h)
}

// clock renders a minute-of-day in the chosen format.
func clock(minutes int, use12 bool) string {
	h, m := minutes/60, minutes%60
	if !use12 {
		return fmt.Sprintf("%02d:%02d", h%24, m)
	}
	suffix := "AM"
	hh := h % 24
	if hh >= 12 {
		suffix = "PM"
	}
	hh %= 12
	if hh == 0 {
		hh = 12
	}
	return fmt.Sprintf("%d:%02d %s", hh, m, suffix)
}

func clockMinute(t time.Time, use12 bool) string {
	return clock(t.Hour()*60+t.Minute(), use12)
}
