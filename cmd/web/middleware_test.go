package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/satyaorz/snippetbox/internal/assert"
)

func TestCommonHeaders(t *testing.T) {
	rr := httptest.NewRecorder()

	r, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// A mock handler that writes a 200 status and an "OK" body, so we can
	// check commonHeaders calls through to the next handler.
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	commonHeaders(next).ServeHTTP(rr, r)

	rs := rr.Result()

	assert.Equal(t, rs.Header.Get("Content-Security-Policy"),
		"default-src 'self'; style-src 'self' fonts.googleapis.com; font-src fonts.gstatic.com")
	assert.Equal(t, rs.Header.Get("Referrer-Policy"), "origin-when-cross-origin")
	assert.Equal(t, rs.Header.Get("X-Content-Type-Options"), "nosniff")
	assert.Equal(t, rs.Header.Get("X-Frame-Options"), "deny")
	assert.Equal(t, rs.Header.Get("X-XSS-Protection"), "0")
	assert.Equal(t, rs.Header.Get("Server"), "Go")

	assert.Equal(t, rs.StatusCode, http.StatusOK)

	defer rs.Body.Close()
	body, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, string(bytes.TrimSpace(body)), "OK")
}

func TestRecoverPanic(t *testing.T) {
	app := newTestApplication(t)

	rr := httptest.NewRecorder()
	r, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("oops")
	})

	app.recoverPanic(next).ServeHTTP(rr, r)

	assert.Equal(t, rr.Code, http.StatusInternalServerError)
	assert.Equal(t, rr.Header().Get("Connection"), "close")
	assert.Equal(t, strings.TrimSpace(rr.Body.String()), "Internal Server Error")
}

func TestRecoverPanicDebug(t *testing.T) {
	app := newTestApplication(t)
	app.debug = true

	rr := httptest.NewRecorder()
	r, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("oops")
	})

	app.recoverPanic(next).ServeHTTP(rr, r)

	// In debug mode the panic message and a stack trace are shown.
	assert.Equal(t, rr.Code, http.StatusInternalServerError)
	assert.StringContains(t, rr.Body.String(), "oops")
	assert.StringContains(t, rr.Body.String(), "goroutine")
}
