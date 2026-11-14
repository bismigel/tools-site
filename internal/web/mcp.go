package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/bismigel/tools-site/internal/tool"
)

// Minimal MCP over streamable HTTP: initialize, tools/list, tools/call.
// Same registry, same schemas as /api/tools — no second definition (FR-9.1).
const mcpProtocolVersion = "2024-11-05"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type mcpTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema schema `json:"inputSchema"`
}

type mcpCallParams struct {
	Name      string                     `json:"name"`
	Arguments map[string]json.RawMessage `json:"arguments"`
}

func (s *Server) mcp(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	var req rpcRequest
	if err := dec.Decode(&req); err != nil {
		writeRPC(w, http.StatusBadRequest, rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32700, Message: "parse error"},
		})
		return
	}

	// A request without an id is a notification: accept it, answer nothing.
	if len(req.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch req.Method {
	case "initialize":
		writeRPC(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "tools-site", "version": "1.0.0"},
		}})

	case "tools/list":
		tools := s.cfg.Registry.All(tool.Public)
		out := make([]mcpTool, 0, len(tools))
		for _, t := range tools {
			out = append(out, mcpTool{Name: t.Name, Description: t.Summary, InputSchema: inputSchema(t)})
		}
		writeRPC(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: req.ID,
			Result: map[string]any{"tools": out}})

	case "tools/call":
		s.mcpCall(w, r, req)

	default:
		writeRPC(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: req.ID,
			Error: &rpcError{Code: -32601, Message: fmt.Sprintf("method %q not found", req.Method)}})
	}
}

func (s *Server) mcpCall(w http.ResponseWriter, r *http.Request, req rpcRequest) {
	var params mcpCallParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeRPC(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: req.ID,
				Error: &rpcError{Code: -32602, Message: "invalid params"}})
			return
		}
	}

	t, ok := s.cfg.Registry.Get(params.Name)
	if !ok {
		writeRPC(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: req.ID,
			Error: &rpcError{Code: -32602, Message: fmt.Sprintf("unknown tool %q", params.Name)}})
		return
	}
	// Gated tools need a credential: 401, not 404, because the endpoint is known
	// and the caller is simply not authorised (FR-9.5). Accepted credentials for
	// server-to-server callers are Access service tokens, added at M3b.
	if t.Visibility != tool.Public {
		writeRPC(w, http.StatusUnauthorized, rpcResponse{JSONRPC: "2.0", ID: req.ID,
			Error: &rpcError{Code: -32001, Message: "unauthorized"}})
		return
	}

	for name := range params.Arguments {
		if _, ok := t.Input(name); !ok {
			s.mcpToolError(w, req.ID, fmt.Sprintf("unknown field %q", name))
			return
		}
	}
	vals := make(tool.Values, len(params.Arguments))
	for k, raw := range params.Arguments {
		vals[k] = rawValue(raw)
	}
	clean, errs := t.Clean(vals)
	if len(errs) > 0 {
		for _, in := range t.Inputs {
			if msg, ok := errs[in.Name]; ok {
				s.mcpToolError(w, req.ID, fmt.Sprintf("%s: %s", in.Name, msg))
				return
			}
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.Timeout)
	defer cancel()

	res, err := t.Run(ctx, clean)
	if err != nil {
		s.metrics.toolRun(t.Name, "error")
		s.mcpToolError(w, req.ID, publicMessage(err))
		return
	}
	s.metrics.toolRun(t.Name, "ok")

	writeRPC(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
		"content":           []map[string]string{{"type": "text", "text": resultText(res)}},
		"structuredContent": toResultJSON(res, reqID(r)),
		"isError":           false,
	}})
}

// mcpToolError is an MCP tool-level failure: content with isError, not a
// transport error. Clients are expected to show it, not retry it.
func (s *Server) mcpToolError(w http.ResponseWriter, id json.RawMessage, msg string) {
	writeRPC(w, http.StatusOK, rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []map[string]string{{"type": "text", "text": msg}},
		"isError": true,
	}})
}

func writeRPC(w http.ResponseWriter, status int, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = writeJSONBody(w, resp)
}

// resultText is the agent-facing text for a result. It mirrors
// web/static/js/webmcp.js exactly, so WebMCP and MCP say the same thing for the
// same call.
func resultText(res tool.Result) string {
	parts := make([]string, 0, len(res.Lines)+len(res.Warnings))
	for _, l := range res.Lines {
		if l.Kind == tool.LineKeyValue {
			parts = append(parts, l.Key+": "+l.Value)
			continue
		}
		parts = append(parts, l.Value)
	}
	parts = append(parts, res.Warnings...)
	if len(parts) == 0 {
		parts = append(parts, res.Title)
	}
	return strings.TrimRight(strings.Join(parts, "\n"), "\n")
}
