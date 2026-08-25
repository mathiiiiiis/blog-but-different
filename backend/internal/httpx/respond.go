package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

// APIError carries a status code and the message the frontend reads from
// `detail`, matching the shape the previous stack produced.
type APIError struct {
	Status  int
	Message string
	cause   error
}

func (e *APIError) Error() string { return e.Message }
func (e *APIError) Unwrap() error { return e.cause }

func Errorf(status int, message string) *APIError {
	return &APIError{Status: status, Message: message}
}

func Wrap(status int, message string, cause error) *APIError {
	return &APIError{Status: status, Message: message, cause: cause}
}

func JSON(w http.ResponseWriter, status int, body any) {
	payload, err := json.Marshal(body)
	if err != nil {
		slog.Error("encoding response", "error", err)
		http.Error(w, `{"detail":"Internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func Detail(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"detail": message})
}

// Handler lets route functions return errors instead of writing them.
type Handler func(http.ResponseWriter, *http.Request) error

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := h(w, r)
	if err == nil {
		return
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.Status >= 500 {
			slog.Error("request failed", "path", r.URL.Path, "error", err)
		}
		Detail(w, apiErr.Status, apiErr.Message)
		return
	}

	slog.Error("unhandled error", "method", r.Method, "path", r.URL.Path, "error", err)
	Detail(w, http.StatusInternalServerError, "Internal server error")
}

// DecodeJSON reads a bounded request body into v.
func DecodeJSON(r *http.Request, v any, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBytes))
	if err := dec.Decode(v); err != nil {
		return Errorf(http.StatusBadRequest, "Malformed request body")
	}
	return nil
}
