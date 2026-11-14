package fx

import (
	"errors"
	"testing"
	"time"
)

const sample = `{
  "base": "EUR",
  "date": "2026-11-13",
  "source": "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml",
  "rates": {"EUR": 1.0, "GBP": 0.83559, "USD": 1.1052, "JPY": 180.0}
}`

func TestParseAndConvert(t *testing.T) {
	tbl, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}

	// 1 GBP in USD: rate[USD]/rate[GBP] = 1.1052/0.83559
	got, err := tbl.Convert(1, "GBP", "USD")
	if err != nil {
		t.Fatal(err)
	}
	want := 1.1052 / 0.83559
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("1 GBP = %v USD, want %v", got, want)
	}

	// Same currency is the identity, not an error.
	if got, _ := tbl.Convert(42, "EUR", "EUR"); got != 42 {
		t.Fatalf("EUR->EUR = %v", got)
	}
}

func TestUnsupportedCurrency(t *testing.T) {
	tbl, _ := Parse([]byte(sample))
	if _, err := tbl.Convert(1, "GBP", "NGN"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if _, err := tbl.Convert(1, "XYZ", "GBP"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

func TestParseRejectsBadFiles(t *testing.T) {
	cases := map[string]string{
		"not json":        `{`,
		"base not EUR":    `{"base":"USD","date":"2026-11-13","rates":{"EUR":1,"GBP":0.8}}`,
		"bad date":        `{"base":"EUR","date":"13/11/2026","rates":{"EUR":1,"GBP":0.8}}`,
		"eur missing":     `{"base":"EUR","date":"2026-11-13","rates":{"GBP":0.8,"USD":1.1}}`,
		"eur not 1":       `{"base":"EUR","date":"2026-11-13","rates":{"EUR":2,"GBP":0.8}}`,
		"negative rate":   `{"base":"EUR","date":"2026-11-13","rates":{"EUR":1,"GBP":-0.8}}`,
		"lowercase code":  `{"base":"EUR","date":"2026-11-13","rates":{"EUR":1,"gbp":0.8}}`,
		"too few entries": `{"base":"EUR","date":"2026-11-13","rates":{"EUR":1}}`,
	}
	for name, body := range cases {
		if _, err := Parse([]byte(body)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestEmbeddedRatesAreValid(t *testing.T) {
	tbl, err := Embedded()
	if err != nil {
		t.Fatalf("embedded rates.json is invalid: %v", err)
	}
	if !tbl.Has("GBP") || !tbl.Has("USD") {
		t.Fatal("embedded rates missing GBP/USD")
	}
	if tbl.AgeInDays(time.Now()) > 3650 {
		t.Fatal("embedded rates are implausibly old")
	}
	if len(tbl.Supported()) < 20 {
		t.Fatalf("only %d currencies embedded", len(tbl.Supported()))
	}
}
