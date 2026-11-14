package tool

import (
	"context"
	"testing"

	"github.com/bismigel/tools-site/internal/fx"
)

// NFR-1.1: a pure tool's p95 handler latency must stay under 20 ms. These
// benchmarks are the evidence, run with `make bench`.

func BenchmarkSlugify(b *testing.B) {
	t := NewSlugify()
	in := Values{"text": "Crème Brûlée — Writing With a Full Stomach!"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := t.Run(context.Background(), in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRateConverter(b *testing.B) {
	tbl, err := fx.Embedded()
	if err != nil {
		b.Fatal(err)
	}
	t := NewRateConverter(tbl)
	in := Values{
		"amount": "550", "currency": "GBP", "period": "day", "target": "AED",
		"basis": "contractor", "amount2": "85", "currency2": "EUR", "period2": "hour",
	}
	clean, errs := t.Clean(in)
	if len(errs) != 0 {
		b.Fatalf("validation: %v", errs)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := t.Run(context.Background(), clean); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMeetingPlanner(b *testing.B) {
	t := NewMeetingPlanner()
	in := Values{
		"city1": "Dubai", "city2": "Manila", "city3": "London",
		"date": "2026-11-16", "work_start": "09:00", "work_end": "17:00",
		"length": "60", "format": "12",
	}
	clean, errs := t.Clean(in)
	if len(errs) != 0 {
		b.Fatalf("validation: %v", errs)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := t.Run(context.Background(), clean); err != nil {
			b.Fatal(err)
		}
	}
}
