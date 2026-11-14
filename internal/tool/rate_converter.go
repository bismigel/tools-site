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

// NewRateConverter builds the currency/period tool on top of the embedded rates.
// It is public because it makes no network call at runtime (ADR-0009).
func NewRateConverter(tbl *fx.Table) Tool {
	currencies := tbl.Supported()
	return Tool{
		Name:        "rate_converter",
		Summary:     "Convert a rate between currencies and periods",
		Description: "Normalises one quoted rate to hourly, daily, weekly, monthly and annual, in two currencies, with the annualisation assumptions shown.",
		Visibility:  Public,
		Inputs: []Input{
			{Name: "amount", Label: "Amount", Kind: Number, Required: true, Min: 0, Max: 10000000,
				Help: "The number you were quoted, e.g. 550."},
			{Name: "currency", Label: "Currency", Kind: Select, Required: true, Options: currencies, Default: "GBP"},
			{Name: "period", Label: "Quoted per", Kind: Select, Required: true,
				Options: []string{"hour", "day", "week", "month", "year"}, Default: "day"},
			{Name: "target", Label: "Show in", Kind: Select, Required: true, Options: currencies, Default: "GBP"},
			{Name: "basis", Label: "Working basis", Kind: Select,
				Options: []string{"contractor", "employee", "custom"}, Default: "contractor",
				Help: "The three numbers below are used only when this is custom."},
			{Name: "hours_per_day", Label: "Hours per day", Kind: Number, Min: 1, Max: 24, Default: "8"},
			{Name: "days_per_week", Label: "Days per week", Kind: Number, Min: 1, Max: 7, Default: "5"},
			{Name: "weeks_per_year", Label: "Weeks per year", Kind: Number, Min: 1, Max: 52, Default: "46"},
		},
		Run: runRateConverter(tbl),
	}
}

func runRateConverter(tbl *fx.Table) func(context.Context, Values) (Result, error) {
	return func(_ context.Context, in Values) (Result, error) {
		amount, err := strconv.ParseFloat(in.Get("amount"), 64)
		if err != nil {
			return Result{}, fmt.Errorf("%w: amount must be a number", ErrBadInput)
		}
		if amount < 0.01 {
			return Result{}, fmt.Errorf("%w: amount must be greater than zero", ErrBadInput)
		}
		if amount > 10_000_000 {
			return Result{}, fmt.Errorf("%w: amount is too large", ErrBadInput)
		}

		from, to, period := in.Get("currency"), in.Get("target"), in.Get("period")
		if !tbl.Has(from) || !tbl.Has(to) {
			return Result{}, fmt.Errorf("%w: %q is not a published currency", ErrBadInput, firstUnsupported(tbl, from, to))
		}

		basis, err := basisFor(in)
		if err != nil {
			return Result{}, err
		}
		hoursPerYear := basis.hoursPerDay * basis.daysPerWeek * basis.weeksPerYear

		hourly, err := toHourly(amount, period, basis, hoursPerYear)
		if err != nil {
			return Result{}, err
		}
		annual := hourly * hoursPerYear

		type value struct {
			key, label string
			amount     float64
		}
		values := []value{
			{"hourly", "Hourly", hourly},
			{"daily", "Daily", hourly * basis.hoursPerDay},
			{"weekly", "Weekly", hourly * basis.hoursPerDay * basis.daysPerWeek},
			{"monthly", "Monthly", annual / 12},
			{"annual", "Annual", annual},
		}

		rows := make([][]Cell, 0, len(values))
		jsonValues := make(map[string]map[string]float64, len(values))
		for _, v := range values {
			converted, err := tbl.Convert(v.amount, from, to)
			if err != nil {
				return Result{}, fmt.Errorf("%w: %v", ErrBadInput, err)
			}
			tone := ToneNormal
			if v.key == period {
				tone = ToneEmphasis
			}
			rows = append(rows, []Cell{
				{Text: v.label, Tone: tone},
				{Text: money(from, v.amount), Tone: tone},
				{Text: money(to, converted), Tone: tone},
			})
			jsonValues[v.key] = map[string]float64{
				strings.ToLower(from): round2(v.amount),
				strings.ToLower(to):   round2(converted),
			}
		}

		rate, _ := tbl.Rate(from, to)
		fxLine := fmt.Sprintf("FX: none (same currency) · ECB reference rate, %s · daily, not live", tbl.Date)
		if from != to {
			fxLine = fmt.Sprintf("FX: 1 %s = %s %s · ECB reference rate, %s · daily, not live",
				from, group(round2(rate)), to, tbl.Date)
		}
		warnings := []string{
			fmt.Sprintf("Assumptions: %s = %s hours/year", basis.label, groupWhole(hoursPerYear)),
			fxLine,
		}

		return Result{
			Title: fmt.Sprintf("%s per %s", money(from, amount), period),
			Table: &Table{
				Caption: "Same money, five periods",
				Columns: []string{"Period", from, to},
				Rows:    rows,
			},
			Warnings: warnings,
			JSON: map[string]any{
				"from":   from,
				"to":     to,
				"period": period,
				"basis":  in.Get("basis"),
				"assumptions": map[string]float64{
					"hours_per_day":  basis.hoursPerDay,
					"days_per_week":  basis.daysPerWeek,
					"weeks_per_year": basis.weeksPerYear,
					"hours_per_year": round2(hoursPerYear),
				},
				"fx": map[string]any{
					"rate":   round2(rate),
					"date":   tbl.Date,
					"source": tbl.Source,
					"live":   false,
				},
				"values": jsonValues,
			},
		}, nil
	}
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

func firstUnsupported(tbl *fx.Table, from, to string) string {
	if !tbl.Has(from) {
		return from
	}
	return to
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
