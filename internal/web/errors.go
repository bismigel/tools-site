package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bismigel/tools-site/internal/tool"
)

// maxBody is the request body cap on every POST (spec §7.6).
const maxBody = 64 << 10

type apiErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type apiErrorEnvelope struct {
	Error apiErrorBody `json:"error"`
}

// statusFor maps a sentinel to a status and a stable machine code (FR-3.6).
func statusFor(err error) (int, string) {
	switch {
	case errors.Is(err, tool.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, tool.ErrBadInput):
		return http.StatusBadRequest, "bad_input"
	case errors.Is(err, tool.ErrUpstream):
		return http.StatusBadGateway, "upstream"
	case errors.Is(err, tool.ErrTimeout):
		return http.StatusGatewayTimeout, "timeout"
	case errors.Is(err, tool.ErrBusy):
		return http.StatusServiceUnavailable, "busy"
	case errors.Is(err, tool.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	default:
		return http.StatusInternalServerError, "internal"
	}
}

// publicMessage returns a user-safe message. Internal detail stays in the log.
func publicMessage(err error) string {
	switch {
	case errors.Is(err, tool.ErrBadInput):
		msg := strings.TrimPrefix(err.Error(), tool.ErrBadInput.Error()+": ")
		if msg == "" {
			return "Check the input and try again."
		}
		return strings.ToUpper(msg[:1]) + msg[1:]
	case errors.Is(err, tool.ErrNotFound):
		return "Tool not found."
	case errors.Is(err, tool.ErrTimeout):
		return "That took too long. Try again."
	case errors.Is(err, tool.ErrBusy):
		return "Too many requests for this tool. Try again shortly."
	case errors.Is(err, tool.ErrForbidden):
		return "That request was refused."
	default:
		return "Something went wrong."
	}
}

func writeJSONError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := statusFor(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = writeJSONBody(w, apiErrorEnvelope{apiErrorBody{
		Code:      code,
		Message:   publicMessage(err),
		RequestID: reqID(r),
	}})
}
