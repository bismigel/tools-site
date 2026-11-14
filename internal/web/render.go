package web

import (
	"io"
	"io/fs"
	"text/template"
)

// Renderer parses the template set per render. At this size (four small files,
// ~10 routes) that is well under a millisecond and it removes the dev/prod
// branch entirely: both modes parse from an fs.FS with the same paths.
type Renderer struct {
	fsys fs.FS
}

// NewRenderer wraps the template filesystem (repo root in dev, embedded in prod).
func NewRenderer(fsys fs.FS) *Renderer {
	return &Renderer{fsys: fsys}
}

func (r *Renderer) parse(files ...string) (*template.Template, error) {
	return template.New("layout").Funcs(templateFuncs()).ParseFS(r.fsys, files...)
}

// Page renders a full page: layout + partials + the page's content block.
func (r *Renderer) Page(w io.Writer, page string, data any) error {
	t, err := r.parse(
		"web/templates/layout.html",
		"web/templates/partials/form.html",
		"web/templates/partials/result.html",
		"web/templates/"+page,
	)
	if err != nil {
		return err
	}
	return t.ExecuteTemplate(w, "layout", data)
}

// Partial renders one named partial, the htmx fragment path.
func (r *Renderer) Partial(w io.Writer, name string, data any) error {
	t, err := r.parse("web/templates/partials/" + name + ".html")
	if err != nil {
		return err
	}
	return t.ExecuteTemplate(w, name, data)
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"eqs":      func(a, b string) bool { return a == b },
		"isNowRow": isNowRow,
	}
}
