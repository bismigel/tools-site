package web

import "github.com/bismigel/tools-site/internal/tool"

// Option is one <option> of a select control.
type Option struct {
	Value    string
	Label    string
	Selected bool
}

// Control is the view model for one input. The template decides the markup
// branch from Type; no tool ever supplies markup (FR-1.7).
type Control struct {
	Name        string
	Label       string
	Help        string
	Type        string // text | textarea | number | checkbox | select
	Value       string
	Err         string
	Required    bool
	Options     []Option
	Suggestions []string
	HasErr      bool
	HasDatalist bool
}

// Controls builds the form view model for a tool, carrying values back on
// validation failure.
func Controls(t tool.Tool, v tool.Values, errs map[string]string) []Control {
	out := make([]Control, 0, len(t.Inputs))
	for _, in := range t.Inputs {
		c := Control{
			Name:        in.Name,
			Label:       in.Label,
			Help:        in.Help,
			Type:        in.Kind.String(),
			Value:       v.Get(in.Name),
			Required:    in.Required,
			Suggestions: in.Suggestions,
			HasDatalist: len(in.Suggestions) > 0,
		}
		if msg, ok := errs[in.Name]; ok {
			c.Err = msg
			c.HasErr = true
		}
		for _, opt := range in.Options {
			c.Options = append(c.Options, Option{
				Value:    opt,
				Label:    opt,
				Selected: opt == c.Value,
			})
		}
		out = append(out, c)
	}
	return out
}

// Defaults turns declared Default values into initial form values.
func Defaults(t tool.Tool) tool.Values {
	v := make(tool.Values, len(t.Inputs))
	for _, in := range t.Inputs {
		if in.Default != "" {
			v[in.Name] = in.Default
		}
	}
	return v
}
