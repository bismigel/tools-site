package tool

import (
	"context"
	"strings"
	"testing"

	"github.com/bismigel/tools-site/internal/fx"
)

const testRates = `{
  "base": "EUR",
  "date": "2026-11-13",
  "source": "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml",
  "rates": {"EUR": 1.0, "GBP": 0.83559, "USD": 1.1052}
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

func TestMeetingPlannerOverlap(t *testing.T) {
	res, err := planner(t, Values{
		"cities": "London, New York", "date": "2026-11-16",
		"work_start": "09:00", "work_end": "17:00", "length": "60",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Title, "3 h overlap") {
		t.Errorf("title = %q", res.Title)
	}
	for _, want := range []string{"14:00–17:00 London", "New York 09:00–12:00"} {
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
	if res.Table.Rows[0][0].Text != "00:00" {
		t.Errorf("first row label = %q", res.Table.Rows[0][0].Text)
	}
}

func TestMeetingPlannerNoOverlap(t *testing.T) {
	res, err := planner(t, Values{
		"cities": "London, Sydney", "date": "2026-11-16",
		"work_start": "09:00", "work_end": "17:00", "length": "60",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Title, "No 60-minute overlap") {
		t.Errorf("title = %q", res.Title)
	}
	if len(res.Warnings) == 0 || !strings.Contains(strings.Join(res.Warnings, " "), "No window of 60 minutes") {
		t.Errorf("warnings = %v", res.Warnings)
	}
}

func TestMeetingPlannerDSTWarning(t *testing.T) {
	// US clocks go back on 2026-11-01.
	res, err := planner(t, Values{
		"cities": "London, New York", "date": "2026-11-01",
		"work_start": "09:00", "work_end": "17:00", "length": "60",
	})
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
		"one city":       {"cities": "London", "date": "2026-11-16"},
		"seven cities":   {"cities": "London, New York, Tokyo, Sydney, Dubai, Berlin, Paris", "date": "2026-11-16"},
		"unknown city":   {"cities": "London, Atlantis", "date": "2026-11-16"},
		"duplicate city": {"cities": "London, london", "date": "2026-11-16"},
		"bad date":       {"cities": "London, New York", "date": "16/11/2026"},
		"date too far":   {"cities": "London, New York", "date": "2030-01-01"},
		"inverted hours": {"cities": "London, New York", "date": "2026-11-16", "work_start": "17:00", "work_end": "09:00"},
		"bad time":       {"cities": "London, New York", "date": "2026-11-16", "work_start": "9am"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := planner(t, in); err == nil {
				t.Fatalf("expected an error for %s", name)
			}
		})
	}
}

func TestMeetingPlannerAcceptsIANANames(t *testing.T) {
	res, err := planner(t, Values{
		"cities": "Europe/London, America/New_York", "date": "2026-11-16", "length": "60",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Table.Columns[1] != "London" || res.Table.Columns[2] != "New York" {
		t.Errorf("columns = %v", res.Table.Columns)
	}
}

func TestMeetingPlannerDayNote(t *testing.T) {
	res, err := planner(t, Values{
		"cities": "London, Auckland", "date": "2026-11-16", "length": "60",
	})
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
	res, err := planner(t, Values{"cities": "London, New York", "date": "2026-11-16", "length": "60"})
	if err != nil {
		t.Fatal(err)
	}
	data, ok := res.JSON.(map[string]any)
	if !ok {
		t.Fatal("JSON is not a map")
	}
	overlap, ok := data["overlap"].(map[string]any)
	if !ok || overlap["found"] != true {
		t.Fatalf("overlap = %#v", overlap)
	}
	if overlap["start"] != "14:00" || overlap["hours"] != 3 {
		t.Errorf("overlap = %#v", overlap)
	}
	if hours, ok := data["hours"].([]map[string]any); !ok || len(hours) != 24 {
		t.Errorf("hours = %T", data["hours"])
	}
}
