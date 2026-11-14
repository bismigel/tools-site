package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/bismigel/tools-site/internal/tool"
)

const (
	siteName = "Tools"
	siteDesc = "Small utilities. No account, no JavaScript required."
)

type page struct {
	Title       string
	Description string
	SiteName    string
	SiteDesc    string
	Year        int
	Dev         bool

	Tools    []tool.Tool
	Tool     *tool.Tool
	Controls []Control
	Result   *resultView
	Invalid  bool
	FormErr  string
	CSRF     string
}

type lineView struct {
	Kind  string
	Key   string
	Value string
}

type cellView struct {
	Text string
	Tone string
	Note string
}

type tableView struct {
	Caption string
	Columns []string
	Rows    [][]cellView
}

type resultView struct {
	Title    string
	Lines    []lineView
	Table    *tableView
	Warnings []string
}

func viewResult(res tool.Result) *resultView {
	v := &resultView{Title: res.Title, Warnings: res.Warnings}
	for _, l := range res.Lines {
		v.Lines = append(v.Lines, lineView{Kind: l.Kind.String(), Key: l.Key, Value: l.Value})
	}
	if res.Table != nil {
		t := &tableView{Caption: res.Table.Caption, Columns: res.Table.Columns}
		for _, row := range res.Table.Rows {
			cells := make([]cellView, 0, len(row))
			for _, c := range row {
				cells = append(cells, cellView{Text: c.Text, Tone: c.Tone.String(), Note: c.Note})
			}
			t.Rows = append(t.Rows, cells)
		}
		v.Table = t
	}
	return v
}

func (s *Server) basePage() page {
	return page{
		SiteName: siteName,
		SiteDesc: siteDesc,
		Year:     s.now().Year(),
		Dev:      s.cfg.Dev,
	}
}

func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, name string, p page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.r.Page(w, name, p); err != nil {
		s.log.Error("render page", "page", name, "request_id", reqID(r), "err", err)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
	}
}

func (s *Server) indexFor(v tool.Visibility) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := s.basePage()
		p.Title = "Tools"
		p.Description = "Small utilities with one job each."
		p.Tools = s.cfg.Registry.All(v)
		s.renderPage(w, r, "index.html", p)
	}
}

func (s *Server) toolPageFor(v tool.Visibility) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, ok := s.cfg.Registry.Get(r.PathValue("name"))
		if !ok || t.Visibility != v {
			s.notFound(w, r)
			return
		}
		p := s.basePage()
		p.Title = t.Name
		p.Description = t.Summary
		p.Tool = &t
		p.Controls = Controls(t, Defaults(t), nil)
		if v == tool.Gated && s.cfg.Access != nil {
			p.CSRF = s.cfg.Access.CSRFFor(r)
		}
		s.renderPage(w, r, "tool.html", p)
	}
}

func (s *Server) toolRunFor(v tool.Visibility) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.sameOrigin(r) {
			http.Error(w, "Cross-origin request refused.", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)

		t, ok := s.cfg.Registry.Get(r.PathValue("name"))
		if !ok || t.Visibility != v {
			s.notFound(w, r)
			return
		}

		if err := r.ParseForm(); err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				http.Error(w, "Request too large.", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "Bad request.", http.StatusBadRequest)
			return
		}

		if v == tool.Gated && s.cfg.Access != nil {
			if !s.cfg.Access.CheckCSRF(r, r.PostForm.Get("_csrf")) {
				http.Error(w, "Cross-site request refused.", http.StatusForbidden)
				return
			}
		}

		raw := make(tool.Values, len(t.Inputs))
		for _, in := range t.Inputs {
			raw[in.Name] = r.PostForm.Get(in.Name)
		}
		clean, errs := t.Clean(raw)

		p := s.basePage()
		p.Title = t.Name
		p.Tool = &t
		p.Invalid = len(errs) > 0
		if v == tool.Gated && s.cfg.Access != nil {
			p.CSRF = s.cfg.Access.CSRFFor(r)
		}

		if p.Invalid {
			p.Controls = Controls(t, raw, errs)
			s.respondForm(w, r, p)
			return
		}
		p.Controls = Controls(t, clean, nil)

		ctx, cancel := context.WithTimeout(r.Context(), s.cfg.Timeout)
		defer cancel()

		res, err := t.Run(ctx, clean)
		if err != nil {
			s.metrics.toolRun(t.Name, "error")
			s.log.Info("tool failed",
				"request_id", reqID(r), "tool", t.Name, "err", fmt.Sprint(err))
			// HTML keeps 200 so htmx swaps the message; the JSON path keeps
			// real status codes (spec FR-2.4, §4.3).
			p.Invalid = true
			p.FormErr = publicMessage(err)
			s.respondForm(w, r, p)
			return
		}
		s.metrics.toolRun(t.Name, "ok")
		p.Result = viewResult(res)
		p.Controls = Controls(t, clean, nil)

		if isHTMX(r) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			if err := s.r.Partial(w, "result", p); err != nil {
				s.log.Error("render result", "err", err)
			}
			return
		}
		s.renderPage(w, r, "tool.html", p)
	}
}

// respondForm re-renders the form. With htmx the target is retargeted to the
// wrapper, so a validation message does not nest a form inside #result.
func (s *Server) respondForm(w http.ResponseWriter, r *http.Request, p page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if isHTMX(r) {
		w.Header().Set("HX-Retarget", "#tool-form")
		w.Header().Set("HX-Swap", "outerHTML")
		if err := s.r.Partial(w, "form", p); err != nil {
			s.log.Error("render form", "err", err)
		}
		return
	}
	s.renderPage(w, r, "tool.html", p)
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
