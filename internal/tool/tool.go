package tool

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Sentinel errors. The web layer maps them to status codes; tools should only
// ever return these (wrapped with %w), never ad-hoc errors with user text that
// could leak internals.
var (
	ErrNotFound  = errors.New("tool not found")
	ErrBadInput  = errors.New("bad input")
	ErrUpstream  = errors.New("upstream failure")
	ErrTimeout   = errors.New("timeout")
	ErrBusy      = errors.New("busy")
	ErrForbidden = errors.New("forbidden")
)

// Visibility is a property of the tool, not of a route (ADR-0003).
type Visibility int

const (
	Public Visibility = iota
	Gated
)

func (v Visibility) String() string {
	if v == Gated {
		return "gated"
	}
	return "public"
}

// Kind is the input control type. The HTML control, the JSON Schema and the
// validation rule all derive from it.
type Kind int

const (
	Text Kind = iota
	Textarea
	Number
	Checkbox
	Select
)

func (k Kind) String() string {
	switch k {
	case Textarea:
		return "textarea"
	case Number:
		return "number"
	case Checkbox:
		return "checkbox"
	case Select:
		return "select"
	default:
		return "text"
	}
}

// Input is one declared field of a tool.
type Input struct {
	Name        string   // ^[a-z][a-z0-9_]{0,31}$
	Label       string   // human label, used as the form label
	Kind        Kind     //
	Help        string   // one line under the field
	Required    bool     //
	Min, Max    int      // runes for Text/Textarea; value range for Number (0/0 = unbounded)
	Options     []string // Select only
	Suggestions []string // optional <datalist>; never affects validation (FR-1.11)
	Default     string   //
	Group       string   // optional fieldset legend; consecutive inputs share one
}

// Values is the untrusted, stringly-typed input after form/JSON decoding and
// before Clean.
type Values map[string]string

// Get returns the raw value for a field.
func (v Values) Get(name string) string { return v[name] }

// LineKind is a rendering hint for Result.Lines. There is deliberately no HTML
// kind: a tool never returns markup (FR-1.7).
type LineKind int

const (
	LineText LineKind = iota
	LineKeyValue
	LineCode
	LineWarning
)

func (k LineKind) String() string {
	switch k {
	case LineKeyValue:
		return "key_value"
	case LineCode:
		return "code"
	case LineWarning:
		return "warning"
	default:
		return "text"
	}
}

// Line is one text row of a result.
type Line struct {
	Kind  LineKind
	Key   string // LineKeyValue only
	Value string
}

// Tone is a semantic cell style; the renderer decides what it looks like.
type Tone int

const (
	ToneNormal Tone = iota
	ToneEmphasis
	ToneMuted
	ToneActive // inside a highlighted band (working hours, best slot)
)

func (t Tone) String() string {
	switch t {
	case ToneEmphasis:
		return "emphasis"
	case ToneMuted:
		return "muted"
	case ToneActive:
		return "active"
	default:
		return "normal"
	}
}

// Cell is one table cell. Note is an optional short annotation (e.g. "+1 day").
type Cell struct {
	Text string
	Tone Tone
	Note string
	// Now marks the cell that represents the current hour, so the renderer can
	// highlight its row. It is a fact about time, not a style.
	Now bool
}

// Table is a generic grid result (rate tables, timezone grids). It is a
// primitive, not a per-tool template.
type Table struct {
	Caption string
	Columns []string
	Rows    [][]Cell
	// Numeric marks columns to right-align with tabular figures (money, counts).
	// Same length as Columns, or nil for none.
	Numeric []bool
}

// File is an optional download attached to a result.
type File struct {
	Name  string
	Bytes []byte
}

// Result is what a tool returns. It carries data only.
type Result struct {
	Title    string
	Lines    []Line
	Table    *Table
	Files    []File
	JSON     any
	Warnings []string
}

// Submit says where the input lives. Pure tools with short inputs submit with
// GET, so the URL *is* the answer and can be bookmarked or sent to a colleague
// (NFR-7.1). Tools with a textarea must POST: a 5,000-rune slug request does not
// fit in a URL.
type Submit int

const (
	SubmitPost Submit = iota // default: long text, not shareable by URL
	SubmitGet                // short inputs, shareable, browser history works
)

func (s Submit) String() string {
	if s == SubmitGet {
		return "get"
	}
	return "post"
}

// Tool is the single definition that every surface is derived from.
type Tool struct {
	Name        string // ^[a-z][a-z0-9_]{0,63}$
	Summary     string // one sentence, <=120 runes, no trailing period
	Description string // longer prose, for the API and the page
	Visibility  Visibility
	Submit      Submit
	// DetectTimezone names the input that should default to the browser's IANA
	// zone when JavaScript is available. Empty means "never". The server cannot
	// know a visitor's zone, so this is progressive enhancement only.
	DetectTimezone string
	Inputs         []Input
	Run            func(ctx context.Context, in Values) (Result, error)
}

// DisplayName is the human title shown in the UI. The identifier (Name) keeps
// underscores because it is a URL segment, a JSON key and an MCP tool name; a
// person should read "Rate converter", not "rate_converter".
func (t Tool) DisplayName() string {
	s := strings.ReplaceAll(t.Name, "_", " ")
	if s == "" {
		return t.Name
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// Input looks up a declared field by name.
func (t Tool) Input(name string) (Input, bool) {
	for _, in := range t.Inputs {
		if in.Name == name {
			return in, true
		}
	}
	return Input{}, false
}

var (
	nameRE  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	inputRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
)

const (
	// MaxInputs keeps forms (and the manifest) human-sized.
	MaxInputs = 20
	// MaxSummary is the one-sentence card/manifest limit.
	MaxSummary = 120
)

// Validate checks a tool definition. Registry.Register calls it, so a bad tool
// fails at startup instead of misbehaving later (FR-1.5).
func Validate(t Tool) error {
	if !nameRE.MatchString(t.Name) {
		return fmt.Errorf("tool name %q must match %s", t.Name, nameRE)
	}
	if t.Summary == "" {
		return fmt.Errorf("tool %q: empty summary", t.Name)
	}
	if n := len([]rune(t.Summary)); n > MaxSummary {
		return fmt.Errorf("tool %q: summary is %d runes, max %d", t.Name, n, MaxSummary)
	}
	if strings.HasSuffix(t.Summary, ".") {
		return fmt.Errorf("tool %q: summary must not end with a period", t.Name)
	}
	if t.Run == nil {
		return fmt.Errorf("tool %q: nil Run", t.Name)
	}
	if len(t.Inputs) > MaxInputs {
		return fmt.Errorf("tool %q: %d inputs, max %d", t.Name, len(t.Inputs), MaxInputs)
	}
	seen := make(map[string]bool, len(t.Inputs))
	for _, in := range t.Inputs {
		if !inputRE.MatchString(in.Name) {
			return fmt.Errorf("tool %q: input name %q must match %s", t.Name, in.Name, inputRE)
		}
		if seen[in.Name] {
			return fmt.Errorf("tool %q: duplicate input %q", t.Name, in.Name)
		}
		seen[in.Name] = true
		if in.Label == "" {
			return fmt.Errorf("tool %q: input %q has no label", t.Name, in.Name)
		}
		if t.Submit == SubmitGet && in.Kind == Textarea {
			return fmt.Errorf("tool %q: a GET tool cannot have a textarea (%q); use SubmitPost", t.Name, in.Name)
		}
		if in.Kind == Select && len(in.Options) == 0 {
			return fmt.Errorf("tool %q: select %q has no options", t.Name, in.Name)
		}
		if in.Default != "" && in.Kind == Select && !contains(in.Options, in.Default) {
			return fmt.Errorf("tool %q: select %q default %q is not an option", t.Name, in.Name, in.Default)
		}
		if in.Min > 0 && in.Max > 0 && in.Min > in.Max {
			return fmt.Errorf("tool %q: input %q has Min > Max", t.Name, in.Name)
		}
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// Registry holds the tools, in registration order, and is read-only after
// startup. It is safe for concurrent use because nothing mutates it.
type Registry struct {
	tools map[string]Tool
	order []string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register validates and adds a tool. It is the only mutation point.
func (r *Registry) Register(t Tool) error {
	if err := Validate(t); err != nil {
		return err
	}
	if _, dup := r.tools[t.Name]; dup {
		return fmt.Errorf("tool %q registered twice", t.Name)
	}
	r.tools[t.Name] = t
	r.order = append(r.order, t.Name)
	return nil
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// All returns the tools of one visibility, in registration order (never map
// order, so the index is stable).
func (r *Registry) All(v Visibility) []Tool {
	out := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		if t := r.tools[name]; t.Visibility == v {
			out = append(out, t)
		}
	}
	return out
}

// Len returns the number of registered tools.
func (r *Registry) Len() int { return len(r.order) }
