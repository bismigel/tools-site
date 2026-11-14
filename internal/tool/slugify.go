package tool

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// SlugifyMax is the hard cap in the tool contract (spec §5.1).
const SlugifyMax = 80

// NewSlugify is the first tool: text in, URL slug out. It exists to prove the
// plumbing with something that needs no network, no secret and no config.
func NewSlugify() Tool {
	return Tool{
		Name:        "slugify",
		Summary:     "Turn text into a URL slug",
		Description: "Lowercases, strips accents, replaces anything that is not a letter or digit with a hyphen, collapses repeats and trims to 80 characters.",
		Visibility:  Public,
		Inputs: []Input{
			{
				Name:     "text",
				Label:    "Text",
				Kind:     Textarea,
				Help:     "Paste a title, a heading or a sentence.",
				Required: true,
				Min:      1,
				Max:      5000,
			},
		},
		Run: runSlugify,
	}
}

func runSlugify(_ context.Context, in Values) (Result, error) {
	slug := Slug(in.Get("text"))
	if slug == "" {
		return Result{}, fmt.Errorf("%w: nothing to slugify", ErrBadInput)
	}
	return Result{
		Title: "Slug",
		Lines: []Line{{Kind: LineCode, Value: slug}},
		JSON:  map[string]string{"slug": slug},
	}, nil
}

// Slug implements the normative algorithm in spec §5.1:
//
//  1. NFKD, drop combining marks
//  2. lowercase
//  3. runs outside [a-z0-9] -> "-"
//  4. collapse and trim "-"
//  5. truncate to 80 runes, trim a trailing "-"
//
// Non-Latin scripts therefore produce an empty string, which callers report as
// bad input rather than guessing a transliteration.
func Slug(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		if unicode.Is(unicode.Mn, r) { // combining mark
			continue
		}
		r = unicode.ToLower(r)
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}

	// Collapse repeats and trim, without regexp on every call.
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, "-")

	if n := len([]rune(out)); n > SlugifyMax {
		out = string([]rune(out)[:SlugifyMax])
		out = strings.TrimRight(out, "-")
	}
	return out
}
