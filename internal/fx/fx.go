// Package fx holds the dated ECB reference rates and the cross-rate maths.
//
// The rates are fetched at build time (ADR-0009), shipped inside the binary, and
// never fetched at runtime. That keeps rate_converter public: no egress, no
// allowlist, no secret, and deterministic tests.
package fx

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

//go:embed rates.json
var embedded []byte

// ErrUnsupported is returned for a currency the ECB does not publish.
var ErrUnsupported = errors.New("unsupported currency")

// Table is the on-disk shape: EUR-based units per euro.
type Table struct {
	Base   string             `json:"base"`
	Date   string             `json:"date"`
	Source string             `json:"source"`
	Rates  map[string]float64 `json:"rates"`
}

// Embedded parses the rates compiled into the binary.
func Embedded() (*Table, error) { return Parse(embedded) }

// Parse validates a rate file. A malformed file fails at startup rather than
// producing silently wrong money.
func Parse(b []byte) (*Table, error) {
	var t Table
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("fx: parse: %w", err)
	}
	if t.Base != "EUR" {
		return nil, fmt.Errorf("fx: base must be EUR, got %q", t.Base)
	}
	if _, err := time.Parse("2006-01-02", t.Date); err != nil {
		return nil, fmt.Errorf("fx: bad date %q: %w", t.Date, err)
	}
	if len(t.Rates) < 2 {
		return nil, fmt.Errorf("fx: %d currencies, need at least 2", len(t.Rates))
	}
	if r, ok := t.Rates["EUR"]; !ok || r != 1 {
		return nil, errors.New("fx: EUR must be present and equal to 1.0")
	}
	for cur, r := range t.Rates {
		if len(cur) != 3 || cur != upper(cur) {
			return nil, fmt.Errorf("fx: bad currency code %q", cur)
		}
		if r <= 0 {
			return nil, fmt.Errorf("fx: non-positive rate for %s", cur)
		}
	}
	return &t, nil
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}

// Supported lists currencies in a stable order.
func (t *Table) Supported() []string {
	out := make([]string, 0, len(t.Rates))
	for cur := range t.Rates {
		out = append(out, cur)
	}
	sort.Strings(out)
	return out
}

// Has reports whether a currency is published.
func (t *Table) Has(cur string) bool {
	_, ok := t.Rates[cur]
	return ok
}

// Convert applies the cross-rate. Rates are units per EUR, so
// amount_in_target = amount * rate[target] / rate[source].
func (t *Table) Convert(amount float64, from, to string) (float64, error) {
	rf, ok := t.Rates[from]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrUnsupported, from)
	}
	rt, ok := t.Rates[to]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrUnsupported, to)
	}
	return amount * rt / rf, nil
}

// Rate is the multiplier from one currency to another, for display.
func (t *Table) Rate(from, to string) (float64, error) {
	return t.Convert(1, from, to)
}

// Published is the rate file's date.
func (t *Table) Published() time.Time {
	d, _ := time.Parse("2006-01-02", t.Date)
	return d
}

// AgeInDays is how stale the file is at now. Used for the page warning and the
// file-age alert.
func (t *Table) AgeInDays(now time.Time) int {
	return int(now.UTC().Sub(t.Published().UTC()).Hours() / 24)
}
