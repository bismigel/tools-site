package tool

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/bismigel/tools-site/internal/fx"
)

const testRates = `{
  "base": "EUR",
  "date": "2026-11-13",
  "source": "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml",
  "rates": {"EUR": 1.0, "GBP": 0.83559, "USD": 1.1052, "AED": 4.058847},
  "derived": {"AED": "AED: fixed peg to USD at 3.6725"}
}`

func testRateTool(t *testing.T) Tool {
	t.Helper()
	tbl, err := fx.Parse([]byte(testRates))
	if err != nil {
		t.Fatal(err)
	}
	return NewRateConverter(tbl)
}

func runRate(t *testing.T, tool Tool, in Values) Result {
	t.Helper()
	clean, errs := tool.Clean(in)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	res, err := tool.Run(context.Background(), clean)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

func TestRateConverterContract(t *testing.T) {
	tool := testRateTool(t)

	t.Run("£550/day contractor, GBP", func(t *testing.T) {
		res := runRate(t, tool, Values{
			"amount": "550", "currency": "GBP", "period": "day", "target": "GBP", "basis": "contractor",
		})
		want := map[string]string{
			"Hourly":  "£68.75",
			"Daily":   "£550.00",
			"Weekly":  "£2,750.00",
			"Monthly": "£10,541.67",
			"Annual":  "£126,500.00",
		}
		if got := len(res.Table.Rows); got != 5 {
			t.Fatalf("rows = %d", got)
		}
		for _, row := range res.Table.Rows {
			if exp, ok := want[row[0].Text]; ok && row[1].Text != exp {
				t.Errorf("%s = %q, want %q", row[0].Text, row[1].Text, exp)
			}
		}
		if !strings.Contains(strings.Join(res.Warnings, " "), "1,840 hours/year") {
			t.Errorf("assumptions missing from warnings: %v", res.Warnings)
		}
	})

	t.Run("£550/day in EUR uses the cross rate", func(t *testing.T) {
		res := runRate(t, tool, Values{
			"amount": "550", "currency": "GBP", "period": "day", "target": "EUR", "basis": "contractor",
		})
		var annual string
		for _, row := range res.Table.Rows {
			if row[0].Text == "Annual" {
				annual = row[2].Text
			}
		}
		// 126500 GBP / 0.83559 = 151,390.04 EUR
		if annual != "€151,390.04" {
			t.Errorf("annual EUR = %q", annual)
		}
	})

	t.Run("€85/hour on the employee basis", func(t *testing.T) {
		res := runRate(t, tool, Values{
			"amount": "85", "currency": "EUR", "period": "hour", "target": "EUR", "basis": "employee",
		})
		var annual, daily string
		for _, row := range res.Table.Rows {
			switch row[0].Text {
			case "Annual":
				annual = row[1].Text
			case "Daily":
				daily = row[1].Text
			}
		}
		// 85 * 7.5 * 5 * 47 = 149,812.50
		if annual != "€149,812.50" {
			t.Errorf("annual = %q", annual)
		}
		if daily != "€637.50" {
			t.Errorf("daily = %q", daily)
		}
	})

	t.Run("$150,000/year on the contractor basis", func(t *testing.T) {
		res := runRate(t, tool, Values{
			"amount": "150000", "currency": "USD", "period": "year", "target": "USD", "basis": "contractor",
		})
		if got := res.Table.Rows[0][1].Text; got != "$81.52" {
			t.Errorf("hourly = %q", got)
		}
	})

	t.Run("custom basis changes the annual figure", func(t *testing.T) {
		res := runRate(t, tool, Values{
			"amount": "550", "currency": "GBP", "period": "day", "target": "GBP",
			"basis": "custom", "hours_per_day": "10", "days_per_week": "5", "weeks_per_year": "40",
		})
		// 550/10 = 55/hour; 55*10*5*40 = 110,000
		if got := res.Table.Rows[4][1].Text; got != "£110,000.00" {
			t.Errorf("annual = %q", got)
		}
	})

	t.Run("zero is rejected at run time", func(t *testing.T) {
		clean, errs := tool.Clean(Values{"amount": "0", "currency": "GBP", "period": "day", "target": "GBP"})
		if len(errs) != 0 {
			t.Fatalf("unexpected validation error: %v", errs)
		}
		if _, err := tool.Run(context.Background(), clean); err == nil {
			t.Fatal("expected bad input for zero")
		}
	})

	t.Run("unknown currency is rejected by validation", func(t *testing.T) {
		_, errs := tool.Clean(Values{"amount": "10", "currency": "NGN", "period": "day", "target": "GBP"})
		if _, ok := errs["currency"]; !ok {
			t.Fatalf("expected a currency error, got %v", errs)
		}
	})
}

func TestRateConverterDerivedCurrency(t *testing.T) {
	// AED is not an ECB quote; it is derived from the USD peg.
	tbl, err := fx.Parse([]byte(`{
      "base": "EUR", "date": "2026-11-13", "source": "test",
      "rates": {"EUR": 1.0, "USD": 1.1052, "AED": 4.058847},
      "derived": {"AED": "AED: derived from its fixed peg to the USD at 3.6725"}
    }`))
	if err != nil {
		t.Fatal(err)
	}
	tool := NewRateConverter(tbl)

	res := runRate(t, tool, Values{
		"amount": "1", "currency": "USD", "period": "hour", "target": "AED", "basis": "contractor",
	})
	if got := res.Table.Rows[0][2].Text; got != "AED 3.67" {
		t.Errorf("1 USD = %q, want AED 3.67", got)
	}
	if !strings.Contains(strings.Join(res.Warnings, " "), "fixed peg") {
		t.Errorf("derived currency not explained: %v", res.Warnings)
	}
}

func TestRateMoneyFormatting(t *testing.T) {
	cases := []struct {
		cur  string
		v    float64
		want string
	}{
		{"GBP", 126500, "£126,500.00"},
		{"EUR", 29962.5, "€29,962.50"},
		{"USD", 81.5217, "$81.52"},
		{"JPY", 1000000, "¥1,000,000.00"},
		{"NGN", 1234.5, "NGN 1,234.50"},
	}
	for _, c := range cases {
		if got := money(c.cur, c.v); got != c.want {
			t.Errorf("money(%s, %v) = %q, want %q", c.cur, c.v, got, c.want)
		}
	}
}

func planner(t *testing.T, in Values) (Result, error) {
	t.Helper()
	tool := NewMeetingPlanner()
	clean, errs := tool.Clean(in)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	return tool.Run(context.Background(), clean)
}

// cities builds planner input from an anchor plus one or more cities.
func cities(anchor string, rest ...string) Values {
	v := Values{"city1": anchor}
	for i, c := range rest {
		v[fmt.Sprintf("city%d", i+2)] = c
	}
	return v
}

func TestMeetingPlannerOverlap(t *testing.T) {
	in := cities("London", "New York")
	in["date"] = "2026-11-16"
	in["length"] = "60"
	in["format"] = "24"

	res, err := planner(t, in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"3 hours overlap", "14:00–17:00 London", "09:00–12:00 New York"} {
		if !strings.Contains(res.Title, want) {
			t.Errorf("title missing %q: %q", want, res.Title)
		}
	}
	if len(res.Table.Rows) != 24 {
		t.Errorf("rows = %d", len(res.Table.Rows))
	}
	if got := len(res.Table.Columns); got != 3 {
		t.Errorf("columns = %d", got)
	}
	// The best window *is* the title; only alternatives get a line.
	if len(res.Lines) != 0 {
		t.Errorf("expected no alternative lines for a single window, got %#v", res.Lines)
	}
}

func TestMeetingPlannerTwelveHourClock(t *testing.T) {
	in := cities("London", "New York")
	in["date"] = "2026-11-16"
	in["format"] = "12"

	res, err := planner(t, in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Title, "2:00 PM–5:00 PM London") {
		t.Errorf("title = %q", res.Title)
	}
	if !strings.Contains(res.Title, "9:00 AM–12:00 PM New York") {
		t.Errorf("title = %q", res.Title)
	}
}

func TestMeetingPlannerNoOverlap(t *testing.T) {
	in := cities("London", "Sydney")
	in["date"] = "2026-11-16"

	res, err := planner(t, in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Title, "No 60-minute overlap") {
		t.Errorf("title = %q", res.Title)
	}
	if !strings.Contains(strings.Join(res.Warnings, " "), "No window of 60 minutes") {
		t.Errorf("warnings = %v", res.Warnings)
	}
	if len(res.Lines) != 0 {
		t.Errorf("expected no slot lines, got %#v", res.Lines)
	}
}

func TestMeetingPlannerDSTWarning(t *testing.T) {
	// US clocks go back on 2026-11-01.
	in := cities("London", "New York")
	in["date"] = "2026-11-01"

	res, err := planner(t, in)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(res.Warnings, " ")
	if !strings.Contains(joined, "clocks change") || !strings.Contains(joined, "New York") {
		t.Errorf("expected a New York DST warning, got %v", res.Warnings)
	}
}

func TestMeetingPlannerRejectsBadInput(t *testing.T) {
	cases := map[string]Values{
		"unknown city":   cities("London", "Atlantis"),
		"duplicate city": cities("London", "london"),
		"bad date":       withDate(cities("London", "New York"), "16/11/2026"),
		"date too far":   withDate(cities("London", "New York"), "2030-01-01"),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := planner(t, in); err == nil {
				t.Fatalf("expected an error for %s", name)
			}
		})
	}

	t.Run("one city reaches the run guard", func(t *testing.T) {
		// Clean would fill city2's default, so call Run directly: the contract
		// still refuses a single city.
		if _, err := runMeetingPlanner(context.Background(), Values{"city1": "London"}); err == nil {
			t.Fatal("expected an error for a single city")
		}
	})

	t.Run("inverted working hours", func(t *testing.T) {
		in := withDate(cities("London", "New York"), "2026-11-16")
		in["work_start"] = "17:00"
		in["work_end"] = "09:00"
		if _, err := planner(t, in); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("bad time", func(t *testing.T) {
		in := withDate(cities("London", "New York"), "2026-11-16")
		in["work_start"] = "9am"
		if _, err := planner(t, in); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func withDate(in Values, date string) Values {
	in["date"] = date
	return in
}

func TestMeetingPlannerAcceptsIANANames(t *testing.T) {
	in := withDate(cities("Europe/London", "America/New_York"), "2026-11-16")
	res, err := planner(t, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Table.Columns[1] != "London" || res.Table.Columns[2] != "New York" {
		t.Errorf("columns = %v", res.Table.Columns)
	}
}

func TestMeetingPlannerDayNote(t *testing.T) {
	in := withDate(cities("London", "Auckland"), "2026-11-16")
	res, err := planner(t, in)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, row := range res.Table.Rows {
		for _, cell := range row[2:] {
			if cell.Note == "+1 day" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected a +1 day note for Auckland in the London evening")
	}
}

func TestMeetingPlannerJSON(t *testing.T) {
	in := withDate(cities("London", "New York"), "2026-11-16")
	res, err := planner(t, in)
	if err != nil {
		t.Fatal(err)
	}
	data, ok := res.JSON.(map[string]any)
	if !ok {
		t.Fatal("JSON is not a map")
	}
	slots, ok := data["slots"].([]map[string]any)
	if !ok || len(slots) == 0 {
		t.Fatalf("slots = %#v", data["slots"])
	}
	if slots[0]["anchor_start"] != "14:00" || slots[0]["hours"] != 3 {
		t.Errorf("slot = %#v", slots[0])
	}
	if hours, ok := data["hours"].([]map[string]any); !ok || len(hours) != 24 {
		t.Errorf("hours = %T", data["hours"])
	}
}

func TestRateConverterCompare(t *testing.T) {
	tool := testRateTool(t)
	res := runRate(t, tool, Values{
		"amount": "550", "currency": "GBP", "period": "day", "target": "GBP",
		"basis":   "contractor",
		"amount2": "85", "currency2": "EUR", "period2": "hour",
	})

	if got := strings.Join(res.Table.Columns, ","); got != "Period,Offer A,Offer B" {
		t.Fatalf("columns = %q", got)
	}
	if !strings.Contains(res.Title, "B is 3.3% higher per year") {
		t.Errorf("title = %q", res.Title)
	}
	annual := res.Table.Rows[4]
	// A: 550/day = 126,500/yr. B: 85 EUR/h = 156,400 EUR/yr = 130,686.28 GBP.
	if annual[1].Text != "£126,500.00" {
		t.Errorf("A annual = %q", annual[1].Text)
	}
	if annual[2].Text != "£130,686.28" {
		t.Errorf("B annual = %q", annual[2].Text)
	}
	if len(annual) != 3 {
		t.Errorf("expected three columns, got %d", len(annual))
	}
	if len(res.Lines) != 2 {
		t.Errorf("expected one summary line per offer, got %#v", res.Lines)
	}
}
