package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/bismigel/tools-site/internal/tool"
)

type manifestTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema schema `json:"inputSchema"`
}

type manifest struct {
	Tools []manifestTool `json:"tools"`
}

// manifestFor serves the JSON manifest. The public route lists public tools;
// the gated route (mounted only behind Auth) lists gated ones. Appending gated
// tools to /api/tools for a valid JWT is M3b (FR-3.2).
func (s *Server) manifestFor(v tool.Visibility) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tools := s.cfg.Registry.All(v)
		m := manifest{Tools: make([]manifestTool, 0, len(tools))}
		for _, t := range tools {
			m.Tools = append(m.Tools, manifestTool{
				Name:        t.Name,
				Description: t.Summary,
				InputSchema: inputSchema(t),
			})
		}
		writeJSON(w, http.StatusOK, m)
	}
}

type cellJSON struct {
	Text string `json:"text"`
	Tone string `json:"tone,omitempty"`
	Note string `json:"note,omitempty"`
}

type tableJSON struct {
	Caption string       `json:"caption,omitempty"`
	Columns []string     `json:"columns"`
	Rows    [][]cellJSON `json:"rows"`
}

type lineJSON struct {
	Kind  string `json:"kind"`
	Key   string `json:"key,omitempty"`
	Value string `json:"value"`
}

type resultJSON struct {
	Title     string     `json:"title"`
	Lines     []lineJSON `json:"lines,omitempty"`
	Table     *tableJSON `json:"table,omitempty"`
	Warnings  []string   `json:"warnings,omitempty"`
	Data      any        `json:"data,omitempty"`
	RequestID string     `json:"request_id"`
}

func toResultJSON(res tool.Result, requestID string) resultJSON {
	out := resultJSON{Title: res.Title, Warnings: res.Warnings, Data: res.JSON, RequestID: requestID}
	for _, l := range res.Lines {
		out.Lines = append(out.Lines, lineJSON{Kind: l.Kind.String(), Key: l.Key, Value: l.Value})
	}
	if res.Table != nil {
		t := &tableJSON{Caption: res.Table.Caption, Columns: res.Table.Columns}
		for _, row := range res.Table.Rows {
			cells := make([]cellJSON, 0, len(row))
			for _, c := range row {
				cells = append(cells, cellJSON{Text: c.Text, Tone: c.Tone.String(), Note: c.Note})
			}
			t.Rows = append(t.Rows, cells)
		}
		out.Table = t
	}
	return out
}

// apiRunFor is the JSON execution path. Unknown fields, bad numbers and wrong
// select values are all bad_input (spec §3.3); unknown tool or a gated tool on
// the public zone is 404, never 403 (FR-3.5).
func (s *Server) apiRunFor(v tool.Visibility) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.sameOrigin(r) {
			writeJSONError(w, r, fmt.Errorf("%w: cross-origin", tool.ErrForbidden))
			return
		}
		if v == tool.Gated && s.cfg.Access != nil && !s.cfg.Access.CheckCSRF(r, "") {
			writeJSONError(w, r, fmt.Errorf("%w: missing CSRF token", tool.ErrForbidden))
			return
		}

		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
		var raw map[string]json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				writeTooLarge(w, r)
				return
			}
			writeJSONError(w, r, fmt.Errorf("%w: malformed JSON body", tool.ErrBadInput))
			return
		}
		if dec.More() {
			writeJSONError(w, r, fmt.Errorf("%w: trailing data after JSON object", tool.ErrBadInput))
			return
		}

		t, ok := s.cfg.Registry.Get(r.PathValue("name"))
		if !ok || t.Visibility != v {
			writeJSONError(w, r, tool.ErrNotFound)
			return
		}
		for k := range raw {
			if _, ok := t.Input(k); !ok {
				writeJSONError(w, r, fmt.Errorf("%w: unknown field %q", tool.ErrBadInput, k))
				return
			}
		}

		vals := make(tool.Values, len(raw))
		for k, rv := range raw {
			vals[k] = rawValue(rv)
		}
		clean, errs := t.Clean(vals)
		if len(errs) > 0 {
			// Deterministic: first error in declaration order.
			for _, in := range t.Inputs {
				if msg, ok := errs[in.Name]; ok {
					writeJSONError(w, r, fmt.Errorf("%w: %s: %s", tool.ErrBadInput, in.Name, msg))
					return
				}
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), s.cfg.Timeout)
		defer cancel()

		res, err := t.Run(ctx, clean)
		if err != nil {
			s.metrics.toolRun(t.Name, "error")
			s.log.Info("tool failed", "request_id", reqID(r), "tool", t.Name, "err", fmt.Sprint(err))
			writeJSONError(w, r, err)
			return
		}
		s.metrics.toolRun(t.Name, "ok")
		writeJSON(w, http.StatusOK, toResultJSON(res, reqID(r)))
	}
}

func writeTooLarge(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	_ = writeJSONBody(w, apiErrorEnvelope{apiErrorBody{
		Code:      "too_large",
		Message:   "Request too large.",
		RequestID: reqID(r),
	}})
}

// rawValue normalises a JSON scalar to the stringly Values type. Numbers, bools
// and strings all become their textual form; null and empty become "".
func rawValue(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	if s[0] == '"' {
		var out string
		if err := json.Unmarshal(raw, &out); err == nil {
			return out
		}
	}
	return s
}
