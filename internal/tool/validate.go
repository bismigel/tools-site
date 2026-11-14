package tool

import (
	"fmt"
	"strconv"
	"strings"
)

// Clean trims and validates raw input against the tool's Input rules, returning
// normalised values and per-field messages. It never returns an error: the
// caller decides whether messages mean 200 + form re-render (HTML) or 400
// (JSON).
//
// Rules (spec §3.3): required, rune length for text, numeric range, select
// membership. Run functions may assume their input is clean.
func (t Tool) Clean(v Values) (Values, map[string]string) {
	out := make(Values, len(t.Inputs))
	errs := make(map[string]string)

	for _, in := range t.Inputs {
		raw := strings.TrimSpace(v[in.Name])

		switch in.Kind {
		case Checkbox:
			if raw == "on" || raw == "true" || raw == "1" {
				out[in.Name] = "true"
			} else {
				out[in.Name] = ""
			}
			if in.Required && out[in.Name] != "true" {
				errs[in.Name] = "This must be checked"
			}

		case Number:
			if raw == "" {
				if in.Required {
					errs[in.Name] = "Required"
				}
				break
			}
			f, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				errs[in.Name] = "Must be a number"
				break
			}
			if in.Min != 0 && f < float64(in.Min) {
				errs[in.Name] = fmt.Sprintf("Must be at least %d", in.Min)
			}
			if in.Max != 0 && f > float64(in.Max) {
				errs[in.Name] = fmt.Sprintf("Must be at most %d", in.Max)
			}
			out[in.Name] = strconv.FormatFloat(f, 'f', -1, 64)

		case Select:
			if raw == "" {
				if in.Required {
					errs[in.Name] = "Required"
				}
				break
			}
			if !contains(in.Options, raw) {
				errs[in.Name] = "Choose one of the listed options"
				break
			}
			out[in.Name] = raw

		default: // Text, Textarea
			n := len([]rune(raw))
			if raw == "" {
				if in.Required {
					errs[in.Name] = "Required"
				}
				break
			}
			if in.Min > 0 && n < in.Min {
				errs[in.Name] = fmt.Sprintf("Must be at least %d characters", in.Min)
			}
			if in.Max > 0 && n > in.Max {
				errs[in.Name] = fmt.Sprintf("Must be at most %d characters", in.Max)
			}
			out[in.Name] = raw
		}
	}
	return out, errs
}
