package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// envelope wraps JSON responses in a top-level object, e.g.
// {"snippet": {...}}, which leaves room to add fields later.
type envelope map[string]any

// isAPIRequest reports whether the request is for the JSON API.
func isAPIRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/")
}

// writeJSON sends data as an indented JSON response.
func (app *application) writeJSON(w http.ResponseWriter, status int, data any, headers http.Header) error {
	js, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	js = append(js, '\n')

	for key, value := range headers {
		w.Header()[key] = value
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)

	return nil
}

// apiError sends a JSON error response: {"error": "message"}.
func (app *application) apiError(w http.ResponseWriter, r *http.Request, status int, message string) {
	err := app.writeJSON(w, status, envelope{"error": message}, nil)
	if err != nil {
		app.logger.Error(err.Error(), "request_id", requestIDFrom(r))
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// apiValidationError sends a 422 with the error message for each field.
func (app *application) apiValidationError(w http.ResponseWriter, r *http.Request, fieldErrors map[string]string) {
	err := app.writeJSON(w, http.StatusUnprocessableEntity, envelope{"error": "validation failed", "fields": fieldErrors}, nil)
	if err != nil {
		app.logger.Error(err.Error(), "request_id", requestIDFrom(r))
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// maxJSONBytes caps request bodies; it comfortably fits a 64KB snippet even
// after JSON escaping.
const maxJSONBytes = 1 << 20

// readJSON decodes a JSON request body into dst, turning the decoder's
// errors into messages that are safe and useful to show API clients.
func (app *application) readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(dst)
	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var maxBytesError *http.MaxBytesError
		var invalidUnmarshalError *json.InvalidUnmarshalError

		switch {
		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)
		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains badly-formed JSON")
		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return fmt.Errorf("body contains the wrong JSON type for field %q", unmarshalTypeError.Field)
			}
			return fmt.Errorf("body contains the wrong JSON type (at character %d)", unmarshalTypeError.Offset)
		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			return fmt.Errorf("body contains unknown field %s", strings.TrimPrefix(err.Error(), "json: unknown field "))
		case errors.As(err, &maxBytesError):
			return fmt.Errorf("body must not be larger than %d bytes", maxBytesError.Limit)
		case errors.As(err, &invalidUnmarshalError):
			// A programming error (passing a non-pointer), not a client error.
			panic(err)
		default:
			return err
		}
	}

	if dec.More() {
		return errors.New("body must only contain a single JSON value")
	}

	return nil
}
