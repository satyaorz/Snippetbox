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
	app.config.debug = true

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

func TestClientIP(t *testing.T) {
	tests := []struct {
		name        string
		behindProxy bool
		remoteAddr  string
		xff         []string
		want        string
	}{
		{name: "Direct", remoteAddr: "203.0.113.7:51234", want: "203.0.113.7"},
		{name: "Direct ignores X-Forwarded-For", remoteAddr: "203.0.113.7:51234", xff: []string{"1.2.3.4"}, want: "203.0.113.7"},
		{name: "Behind proxy", behindProxy: true, remoteAddr: "10.0.0.2:1234", xff: []string{"198.51.100.9"}, want: "198.51.100.9"},
		{name: "Spoofed entries are ignored", behindProxy: true, remoteAddr: "10.0.0.2:1234", xff: []string{"1.1.1.1, 198.51.100.9"}, want: "198.51.100.9"},
		{name: "Multiple headers", behindProxy: true, remoteAddr: "10.0.0.2:1234", xff: []string{"1.1.1.1", "198.51.100.9"}, want: "198.51.100.9"},
		{name: "Garbage falls back to peer", behindProxy: true, remoteAddr: "10.0.0.2:1234", xff: []string{"not-an-ip"}, want: "10.0.0.2"},
		{name: "No header falls back to peer", behindProxy: true, remoteAddr: "10.0.0.2:1234", want: "10.0.0.2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApplication(t)
			app.config.behindProxy = tt.behindProxy

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			for _, v := range tt.xff {
				r.Header.Add("X-Forwarded-For", v)
			}

			assert.Equal(t, app.clientIP(r), tt.want)
		})
	}
}

func TestRequestID(t *testing.T) {
	run := func(behindProxy bool, incoming string) (header, inContext string) {
		app := newTestApplication(t)
		app.config.behindProxy = behindProxy

		rr := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if incoming != "" {
			r.Header.Set("X-Request-Id", incoming)
		}

		app.requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			inContext = requestIDFrom(r)
		})).ServeHTTP(rr, r)

		return rr.Header().Get("X-Request-Id"), inContext
	}

	header, ctxID := run(false, "")
	assert.Equal(t, len(header), 16)
	assert.Equal(t, ctxID, header)

	// A client can't choose its own ID unless we're behind a trusted proxy...
	header, _ = run(false, "from-client")
	assert.Equal(t, header == "from-client", false)

	// ...and even then only well-formed IDs are accepted.
	header, _ = run(true, "proxy-id-123")
	assert.Equal(t, header, "proxy-id-123")

	header, _ = run(true, "bad id\nwith newline")
	assert.Equal(t, len(header), 16)
}
