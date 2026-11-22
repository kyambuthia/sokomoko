package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

type responseEnvelope struct {
	Data any `json:"data"`
}

type errorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	if status == http.StatusNoContent {
		return
	}
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}

func writeData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, responseEnvelope{Data: data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message}})
}

func methodNotAllowed(w http.ResponseWriter, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
}

type headResponseWriter struct {
	http.ResponseWriter
}

func (headResponseWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func requireGetOrHead(w http.ResponseWriter, r *http.Request) (http.ResponseWriter, bool) {
	switch r.Method {
	case http.MethodGet:
		return w, true
	case http.MethodHead:
		return headResponseWriter{ResponseWriter: w}, true
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodHead)
		return w, false
	}
}
