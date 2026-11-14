package web

import (
	"strconv"

	"github.com/bismigel/tools-site/internal/tool"
)

// JSON Schema generated from []Input. This is the manifest's inputSchema and
// the same object MCP advertises (FR-9.3): one generator, one output.
type schema struct {
	Type       string              `json:"type"`
	Properties map[string]property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

type property struct {
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description,omitempty"`
	MinLength   *int     `json:"minLength,omitempty"`
	MaxLength   *int     `json:"maxLength,omitempty"`
	Minimum     *float64 `json:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
}

func inputSchema(t tool.Tool) schema {
	s := schema{
		Type:       "object",
		Properties: make(map[string]property, len(t.Inputs)),
	}
	for _, in := range t.Inputs {
		p := property{Description: in.Help}
		switch in.Kind {
		case tool.Number:
			p.Type = "number"
			if in.Min != 0 {
				v := float64(in.Min)
				p.Minimum = &v
			}
			if in.Max != 0 {
				v := float64(in.Max)
				p.Maximum = &v
			}
			if in.Default != "" {
				if f, err := strconv.ParseFloat(in.Default, 64); err == nil {
					p.Default = f
				}
			}
		case tool.Checkbox:
			p.Type = "boolean"
			if in.Default == "true" {
				p.Default = true
			}
		case tool.Select:
			p.Type = "string"
			p.Enum = in.Options
			if in.Default != "" {
				p.Default = in.Default
			}
		default: // Text, Textarea
			p.Type = "string"
			if in.Min > 0 {
				v := in.Min
				p.MinLength = &v
			}
			if in.Max > 0 {
				v := in.Max
				p.MaxLength = &v
			}
			if in.Default != "" {
				p.Default = in.Default
			}
		}
		s.Properties[in.Name] = p
		if in.Required {
			s.Required = append(s.Required, in.Name)
		}
	}
	return s
}
