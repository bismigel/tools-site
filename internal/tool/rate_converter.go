package tool

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bismigel/tools-site/internal/fx"
)

type rateBasis struct {
	hoursPerDay  float64
	daysPerWeek  float64
	weeksPerYear float64
	label        string
}

var rateBases = map[string]rateBasis{
	"contractor": {8, 5, 46, "contractor · 8 h/day · 5 d/week · 46 wk/year"},
	"employee":   {7.5, 5, 47, "employee · 7.5 h/day · 5 d/week · 47 wk/year (52 less 25 days leave)"},
}

var ratePeriods = []string{"hour", "day", "week", "month", "year"}

// NewRateConverter normalises a rate to five periods, in two currencies, and can
// compare a second offer side by side. Public: the rates are build-time data
// (ADR-0009), so there is no runtime fetch.
func NewRateConverter(tbl *fx.Table) Tool {
	currencies := tbl.Supported()
	return Tool{
		Name:        "rate_converter",
		Summary:     "Convert a rate between currencies and periods",
		Description: "Normalises one quoted rate to hourly, daily, weekly, monthly and annual, in two currencies, with the annualisation assumptions shown — and compares two offers side by side.",
		Visibility:  Public,
		Submit:      SubmitGet,
		Inputs: []Input{
			{Name: "amount", Label: "Amount", Kind: Number, Required: true, Max: 10000000,
				Group: "The rate you were quoted",
				Help:  "e.g. 550"},
			{Name: "currency", Label: "Currency", Kind: Select, Required: true, Options: currencies, Default: "AED",
				Group: "The rate you were quoted"},
			{Name: "period", Label: "Quoted per", Kind: Select, Required: true, Options: ratePeriods, Default: "day",
				Group: "The rate you were quoted"},
			{Name: "target", Label: "Convert to", Kind: Select, Required: true, Options: currencies, Default: "AED",
				Group: "The rate you were quoted"},
			{Name: "basis", Label: "Basis", Kind: Select,
				Options: []string{"contractor", "employee", "custom"}, Default: "contractor",
				Group: "Working basis",
				Help:  "Contractor is 8h × 5d × 46wk. The numbers below apply when this is custom."},
			{Name: "hours_per_day", Label: "Hours per day", Kind: Number, Min: 1, Max: 24, Default: "8",
				Group: "Working basis"},
			{Name: "days_per_week", Label: "Days per week", Kind: Number, Min: 1, Max: 7, Default: "5",
				Group: "Working basis"},
			{Name: "weeks_per_year", Label: "Weeks per year", Kind: Number, Min: 1, Max: 52, Default: "46",
				Group: "Working basis"},
			{Name: "amount2", Label: "Second offer amount", Kind: Number, Max: 10000000,
				Group: "Compare a second offer (optional)",
				Help:  "Leave empty to skip the comparison."},
			{Name: "currency2", Label: "Second offer currency", Kind: Select, Options: currencies, Default: "AED",
				Group: "Compare a second offer (optional)"},
			{Name: "period2", Label: "Second offer quoted per", Kind: Select, Options: ratePeriods, Default: "day",
				Group: "Compare a second offer (optional)"},
		},
		Run: runRateConverter(tbl),
	}
}

type offer struct {
	amount    float64
	currency  string
	period    string
	source    map[string]float64 // period -> amount in its own currency
	converted map[string]float64 // period -> amount in the target currency
}

var periodLabels = map[string]string{
	"hour": "Hourly", "day": "Daily", "week": "Weekly", "month": "Monthly", "year": "Annual",
}

func runRateConverter(tbl *fx.Table) func(context.Context, Values) (Result, error) {
	return func(_ context.Context, in Values) (Result, error) {
		basis, err := basisFor(in)
		if err != nil {
			return Result{}, err
		}
		hoursPerYear := basis.hoursPerDay * basis.daysPerWeek * basis.weeksPerYear
		target := in.Get("target")
		if !tbl.Has(target) {
			return Result{}, fmt.Errorf("%w: %q is not a published currency", ErrBadInput, target)
		}

		a, err := buildOffer(tbl, in.Get("amount"), in.Get("currency"), in.Get("period"), target, basis, hoursPerYear)
		if err != nil {
			return Result{}, err
		}

		lines := []Line{{Kind: LineText, Value: summaryLine(a, target)}}
		warnings := []string{
			fmt.Sprintf("Assumptions: %s = %s hours/year", basis.label, groupWhole(hoursPerYear)),
		}
		seenFX := make(map[string]bool, 2)
		addFX := func(from, to string) {
			if from == to || seenFX[from+">"+to] {
				return
			}
			seenFX[from+">"+to] = true
			warnings = append(warnings, fxLine(tbl, from, to))
		}
		addFX(a.currency, target)

		var table *Table
		var data map[string]any
		title := fmt.Sprintf("%s per %s", money(a.currency, a.amount), a.period)
		comparing := strings.TrimSpace(in.Get("amount2")) != ""

		if comparing {
			b, err := buildOffer(tbl, in.Get("amount2"), in.Get("currency2"), in.Get("period2"), target, basis, hoursPerYear)
			if err != nil {
				return Result{}, err
			}
			lines = append(lines, Line{Kind: LineText, Value: summaryLine(b, target)})

			table = compareTable(a, b, target)
			addFX(b.currency, target)

			delta := 0.0
			if a.converted["year"] > 0 {
				delta = (b.converted["year"] - a.converted["year"]) / a.converted["year"] * 100
			}
			title = fmt.Sprintf("%s per %s vs %s per %s — B is %s",
				money(a.currency, a.amount), a.period, money(b.currency, b.amount), b.period,
				deltaPhrase(delta))
			data = map[string]any{
				"a":     offerJSON(a),
				"b":     offerJSON(b),
				"delta": map[string]any{"annual_percent": round2(delta)},
			}
		} else {
			table = singleTable(a, target)
			data = map[string]any{"a": offerJSON(a)}
		}
		if len(seenFX) == 0 {
			warnings = append(warnings, fxLine(tbl, target, target))
		}
		used := []string{a.currency, target}
		if comparing {
			used = append(used, in.Get("currency2"))
		}
		for _, cur := range used {
			if note, ok := tbl.IsDerived(cur); ok {
				warnings = append(warnings, note)
			}
		}

		return Result{
			Title:    title,
			Lines:    lines,
			Table:    table,
			Warnings: dedupe(warnings),
			JSON: map[string]any{
				"target":      target,
				"basis":       in.Get("basis"),
				"assumptions": assumptionsJSON(basis, hoursPerYear),
				"fx":          fxJSON(tbl, a.currency, target),
				"offers":      data,
			},
		}, nil
	}
}

func buildOffer(tbl *fx.Table, amountRaw, currency, period, target string, b rateBasis, hoursPerYear float64) (offer, error) {
	amount, err := strconv.ParseFloat(strings.TrimSpace(amountRaw), 64)
	if err != nil {
		return offer{}, fmt.Errorf("%w: amount must be a number", ErrBadInput)
	}
	if amount < 0.01 {
		return offer{}, fmt.Errorf("%w: amount must be greater than zero", ErrBadInput)
	}
	if amount > 10_000_000 {
		return offer{}, fmt.Errorf("%w: amount is too large", ErrBadInput)
	}
	if !tbl.Has(currency) {
		return offer{}, fmt.Errorf("%w: %q is not a published currency", ErrBadInput, currency)
	}
	if !tbl.Has(target) {
		return offer{}, fmt.Errorf("%w: %q is not a published currency", ErrBadInput, target)
	}

	hourly, err := toHourly(amount, period, b, hoursPerYear)
	if err != nil {
		return offer{}, err
	}
	annual := hourly * hoursPerYear

	o := offer{
		amount:    amount,
		currency:  currency,
		period:    period,
		source:    make(map[string]float64, 5),
		converted: make(map[string]float64, 5),
	}
	values := map[string]float64{
		"hour":  hourly,
		"day":   hourly * b.hoursPerDay,
		"week":  hourly * b.hoursPerDay * b.daysPerWeek,
		"month": annual / 12,
		"year":  annual,
	}
	for k, v := range values {
		converted, err := tbl.Convert(v, currency, target)
		if err != nil {
			return offer{}, fmt.Errorf("%w: %v", ErrBadInput, err)
		}
		o.source[k] = v
		o.converted[k] = converted
	}
	return o, nil
}

// summaryLine is the answer in words before the table: the number a recruiter
// would say out loud.
func summaryLine(o offer, target string) string {
	annual := money(target, o.converted["year"])
	monthly := money(target, o.converted["month"])
	if o.currency == target {
		return fmt.Sprintf("%s per %s = %s per year = %s per month",
			money(o.currency, o.amount), o.period, annual, monthly)
	}
	return fmt.Sprintf("%s per %s = %s per year = %s per month (at today's reference rate)",
		money(o.currency, o.amount), o.period, annual, monthly)
}

func singleTable(a offer, target string) *Table {
	rows := make([][]Cell, 0, 5)
	for _, p := range ratePeriods {
		tone := ToneNormal
		if p == a.period {
			tone = ToneEmphasis
		}
		rows = append(rows, []Cell{
			{Text: periodLabels[p]},
			{Text: money(a.currency, a.source[p]), Tone: tone},
			{Text: money(target, a.converted[p]), Tone: tone},
		})
	}
	return &Table{
		Caption: "Same money, five periods",
		Columns: []string{"Period", a.currency, target},
		Numeric: []bool{false, true, true},
		Rows:    rows,
	}
}

func compareTable(a, b offer, target string) *Table {
	rows := make([][]Cell, 0, 5)
	for _, p := range ratePeriods {
		toneA, toneB := ToneNormal, ToneNormal
		if p == a.period {
			toneA = ToneEmphasis
		}
		if p == b.period {
			toneB = ToneEmphasis
		}
		rows = append(rows, []Cell{
			{Text: periodLabels[p]},
			{Text: money(target, a.converted[p]), Tone: toneA},
			{Text: money(target, b.converted[p]), Tone: toneB},
		})
	}
	return &Table{
		Caption: fmt.Sprintf("Both offers in %s", target),
		Columns: []string{"Period", "Offer A", "Offer B"},
		Numeric: []bool{false, true, true},
		Rows:    rows,
	}
}

// deltaPhrase is the one-line verdict a recruiter would say: "3.3% higher".
func deltaPhrase(delta float64) string {
	switch {
	case delta > 0.05:
		return fmt.Sprintf("%.1f%% higher per year", delta)
	case delta < -0.05:
		return fmt.Sprintf("%.1f%% lower per year", -delta)
	default:
		return "about the same per year"
	}
}

func offerJSON(o offer) map[string]any {
	values := make(map[string]map[string]float64, 5)
	for _, p := range ratePeriods {
		values[p] = map[string]float64{
			strings.ToLower(o.currency): round2(o.source[p]),
			"target":                    round2(o.converted[p]),
		}
	}
	return map[string]any{
		"amount":   round2(o.amount),
		"currency": o.currency,
		"period":   o.period,
		"values":   values,
	}
}

func assumptionsJSON(b rateBasis, hoursPerYear float64) map[string]float64 {
	return map[string]float64{
		"hours_per_day":  b.hoursPerDay,
		"days_per_week":  b.daysPerWeek,
		"weeks_per_year": b.weeksPerYear,
		"hours_per_year": round2(hoursPerYear),
	}
}

func fxJSON(tbl *fx.Table, from, to string) map[string]any {
	rate, _ := tbl.Rate(from, to)
	return map[string]any{
		"from": from, "to": to,
		"rate": round2(rate), "date": tbl.Date, "source": tbl.Source, "live": false,
	}
}

func fxLine(tbl *fx.Table, from, to string) string {
	if from == to {
		return fmt.Sprintf("FX: none (same currency) · ECB reference rate, %s · daily, not live", tbl.Date)
	}
	rate, _ := tbl.Rate(from, to)
	return fmt.Sprintf("FX: 1 %s = %s %s · ECB reference rate, %s · daily, not live",
		from, group(round2(rate)), to, tbl.Date)
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func basisFor(in Values) (rateBasis, error) {
	switch in.Get("basis") {
	case "", "contractor":
		return rateBases["contractor"], nil
	case "employee":
		return rateBases["employee"], nil
	case "custom":
		hpd, err := number(in, "hours_per_day", 1, 24)
		if err != nil {
			return rateBasis{}, err
		}
		dpw, err := number(in, "days_per_week", 1, 7)
		if err != nil {
			return rateBasis{}, err
		}
		wpy, err := number(in, "weeks_per_year", 1, 52)
		if err != nil {
			return rateBasis{}, err
		}
		return rateBasis{hpd, dpw, wpy, "custom"}, nil
	default:
		return rateBasis{}, fmt.Errorf("%w: unknown basis %q", ErrBadInput, in.Get("basis"))
	}
}

func number(in Values, name string, min, max float64) (float64, error) {
	v, err := strconv.ParseFloat(in.Get(name), 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %s must be a number", ErrBadInput, name)
	}
	if v < min || v > max {
		return 0, fmt.Errorf("%w: %s must be between %s and %s", ErrBadInput, name, group(min), group(max))
	}
	return v, nil
}

func toHourly(amount float64, period string, b rateBasis, hoursPerYear float64) (float64, error) {
	switch period {
	case "hour":
		return amount, nil
	case "day":
		return amount / b.hoursPerDay, nil
	case "week":
		return amount / (b.hoursPerDay * b.daysPerWeek), nil
	case "month":
		return amount / (hoursPerYear / 12), nil
	case "year":
		return amount / hoursPerYear, nil
	default:
		return 0, fmt.Errorf("%w: unknown period %q", ErrBadInput, period)
	}
}

// symbols are display-only; an unknown currency falls back to its code.
var symbols = map[string]string{
	"AUD": "A$", "BRL": "R$", "CAD": "C$", "CHF": "CHF ", "CNY": "¥", "CZK": "Kč ",
	"DKK": "kr ", "EUR": "€", "GBP": "£", "HKD": "HK$", "HUF": "Ft ", "ILS": "₪",
	"INR": "₹", "ISK": "kr ", "JPY": "¥", "KRW": "₩", "MXN": "MX$", "MYR": "RM ",
	"NOK": "kr ", "NZD": "NZ$", "PHP": "₱", "PLN": "zł ", "RON": "lei ", "SEK": "kr ",
	"SGD": "S$", "THB": "฿", "TRY": "₺", "USD": "$", "ZAR": "R ",
}

func money(cur string, v float64) string {
	sym, ok := symbols[cur]
	if !ok {
		sym = cur + " "
	}
	return sym + group(round2(v))
}

// group formats with two decimals and thousands separators, without a locale.
func group(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatFloat(v, 'f', 2, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	b.WriteByte('.')
	b.WriteString(frac)
	return b.String()
}

// groupWhole formats an integer-valued quantity with thousands separators.
func groupWhole(v float64) string {
	s := strconv.FormatFloat(v, 'f', 0, 64)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func round2(v float64) float64 {
	if v < 0 {
		return -round2(-v)
	}
	return float64(int64(v*100+0.5)) / 100
}
