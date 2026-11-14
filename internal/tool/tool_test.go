package tool

import (
	"context"
	"strings"
	"testing"
)

func mkTool(name string) Tool {
	return Tool{
		Name:       name,
		Summary:    "Does one thing",
		Run:        func(context.Context, Values) (Result, error) { return Result{}, nil },
		Inputs:     []Input{{Name: "text", Label: "Text", Kind: Text}},
		Visibility: Public,
	}
}

func TestRegistryRejectsBadDefinitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		tool Tool
	}{
		{"hyphen in name", func() Tool { x := mkTool("rate-converter"); return x }()},
		{"uppercase name", func() Tool { x := mkTool("Slugify"); return x }()},
		{"empty summary", func() Tool { x := mkTool("ok"); x.Summary = ""; return x }()},
		{"summary ends in period", func() Tool { x := mkTool("ok"); x.Summary = "Does one thing."; return x }()},
		{"summary too long", func() Tool { x := mkTool("ok"); x.Summary = strings.Repeat("a", 121); return x }()},
		{"nil run", func() Tool { x := mkTool("ok"); x.Run = nil; return x }()},
		{"bad input name", func() Tool {
			x := mkTool("ok")
			x.Inputs = []Input{{Name: "Bad-Name", Label: "X"}}
			return x
		}()},
		{"duplicate input", func() Tool {
			x := mkTool("ok")
			x.Inputs = []Input{{Name: "same", Label: "A"}, {Name: "same", Label: "B"}}
			return x
		}()},
		{"select without options", func() Tool {
			x := mkTool("ok")
			x.Inputs = []Input{{Name: "pick", Label: "Pick", Kind: Select}}
			return x
		}()},
		{"select default not an option", func() Tool {
			x := mkTool("ok")
			x.Inputs = []Input{{Name: "pick", Label: "Pick", Kind: Select, Options: []string{"a"}, Default: "b"}}
			return x
		}()},
		{"too many inputs", func() Tool {
			x := mkTool("ok")
			x.Inputs = nil
			for i := 0; i < MaxInputs+1; i++ {
				x.Inputs = append(x.Inputs, Input{Name: "f" + string(rune('a'+i)), Label: "F"})
			}
			return x
		}()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			if err := r.Register(tc.tool); err == nil {
				t.Fatalf("expected Register to reject %q", tc.name)
			}
		})
	}
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if err := r.Register(mkTool("ok")); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(mkTool("ok")); err == nil {
		t.Fatal("expected duplicate registration to fail")
	}
}

func TestRegistryAllIsDeterministic(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	for _, n := range []string{"alpha", "beta", "gamma"} {
		if err := r.Register(mkTool(n)); err != nil {
			t.Fatal(err)
		}
	}
	gated := mkTool("secret")
	gated.Visibility = Gated
	if err := r.Register(gated); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		got := r.All(Public)
		want := []string{"alpha", "beta", "gamma"}
		if len(got) != len(want) {
			t.Fatalf("len=%d", len(got))
		}
		for j := range want {
			if got[j].Name != want[j] {
				t.Fatalf("order changed: %v", got)
			}
		}
	}
	if n := len(r.All(Gated)); n != 1 {
		t.Fatalf("gated count = %d", n)
	}
}

func TestClean(t *testing.T) {
	t.Parallel()

	tl := Tool{Inputs: []Input{
		{Name: "text", Label: "Text", Kind: Textarea, Required: true, Min: 2, Max: 5},
		{Name: "num", Label: "Num", Kind: Number, Min: 1, Max: 10},
		{Name: "pick", Label: "Pick", Kind: Select, Options: []string{"a", "b"}, Required: true},
		{Name: "ok", Label: "OK", Kind: Checkbox},
	}}

	cases := []struct {
		name  string
		in    Values
		field string
		want  string
	}{
		{"required missing", Values{}, "text", "Required"},
		{"too short", Values{"text": "a"}, "text", "at least 2"},
		{"too long", Values{"text": "abcdef"}, "text", "at most 5"},
		{"number below min", Values{"text": "ok", "num": "0", "pick": "a"}, "num", "at least 1"},
		{"number above max", Values{"text": "ok", "num": "11", "pick": "a"}, "num", "at most 10"},
		{"number not a number", Values{"text": "ok", "num": "x", "pick": "a"}, "num", "Must be a number"},
		{"select not listed", Values{"text": "ok", "pick": "zzz"}, "pick", "listed options"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := tl.Clean(tc.in)
			msg, ok := errs[tc.field]
			if !ok {
				t.Fatalf("no error for %s, got %v", tc.field, errs)
			}
			if !strings.Contains(msg, tc.want) {
				t.Fatalf("error %q does not contain %q", msg, tc.want)
			}
		})
	}

	t.Run("valid", func(t *testing.T) {
		got, errs := tl.Clean(Values{"text": "  hi  ", "num": "5", "pick": "b", "ok": "on"})
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if got["text"] != "hi" {
			t.Fatalf("not trimmed: %q", got["text"])
		}
		if got["ok"] != "true" {
			t.Fatalf("checkbox not normalised: %q", got["ok"])
		}
	})
}

// Vectors from spec.md §5.1.
