package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/satyaorz/snippetbox/internal/models"

	"github.com/justinas/nosurf"
)

func commonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; style-src 'self' fonts.googleapis.com; font-src fonts.gstatic.com")

		w.Header().Set("Referrer-Policy", "origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")
		w.Header().Set("X-XSS-Protection", "0")

		w.Header().Set("Server", "Go")

		next.ServeHTTP(w, r)

	})
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.Header().Set("Connection", "close")

				app.serverError(w, r, fmt.Errorf("%s", err))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// requestIDPattern limits which incoming X-Request-Id values we accept from a
// proxy, so a client can't inject arbitrary text into our logs.
var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// requestID gives every request an ID, returned in the X-Request-Id response
// header and attached to its log entries, so a user-reported error can be
// matched to the logs. Behind a trusted proxy we reuse the proxy's ID.
func (app *application) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if !app.config.behindProxy || !requestIDPattern.MatchString(id) {
			b := make([]byte, 8)
			rand.Read(b)
			id = hex.EncodeToString(b)
		}

		w.Header().Set("X-Request-Id", id)

		ctx := context.WithValue(r.Context(), requestIDContextKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// statusRecorder wraps a http.ResponseWriter to remember the status code and
// number of bytes written, for logging.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	size        int
	wroteHeader bool
}

func (sr *statusRecorder) WriteHeader(status int) {
	if !sr.wroteHeader {
		sr.status = status
		sr.wroteHeader = true
	}
	sr.ResponseWriter.WriteHeader(status)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if !sr.wroteHeader {
		sr.WriteHeader(http.StatusOK)
	}
	n, err := sr.ResponseWriter.Write(b)
	sr.size += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (sr *statusRecorder) Unwrap() http.ResponseWriter {
	return sr.ResponseWriter
}

// logRequest logs one line per request once the response has been sent,
// including the status code, response size and how long it took, and updates
// the request metrics.
func (app *application) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		metricRequestsReceived.Add(1)

		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)

		duration := time.Since(start)
		recordResponse(sr.status, duration)

		app.logger.Info("request",
			"request_id", requestIDFrom(r),
			"ip", app.clientIP(r),
			"proto", r.Proto,
			"method", r.Method,
			"uri", r.URL.RequestURI(),
			"status", sr.status,
			"size", sr.size,
			"duration", duration.String(),
		)
	})
}

// requestIDFrom returns the ID set by the requestID middleware, if any.
func requestIDFrom(r *http.Request) string {
	id, _ := r.Context().Value(requestIDContextKey).(string)
	return id
}

// clientIP returns the IP address of the client. Normally that's the address
// of the TCP peer, but behind a trusted proxy it's the last address in
// X-Forwarded-For: the one the proxy itself appended. (Entries further left
// were supplied by the client and can't be trusted.)
func (app *application) clientIP(r *http.Request) string {
	if app.config.behindProxy {
		xff := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
		parts := strings.Split(xff, ",")
		if last := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(last) != nil {
			return last
		}
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// requireAuthentication redirects unauthenticated users to the login page.
func (app *application) requireAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.isAuthenticated(r) {
			// Remember which page the user was trying to reach, so we can send
			// them back there after they log in. Only GET requests are stored,
			// since redirecting to a POST-only route would just give a 405.
			// r.URL.Path has already been cleaned by ServeMux, so it is always
			// a local path and can't be abused as an open redirect.
			if r.Method == http.MethodGet {
				app.sessionManager.Put(r.Context(), "redirectPathAfterLogin", r.URL.Path)
			}

			http.Redirect(w, r, "/user/login", http.StatusSeeOther)
			return
		}

		// Otherwise set the "Cache-Control: no-store" header so that pages
		// which require authentication are not stored in the users browser
		// cache (or other intermediary cache).
		w.Header().Add("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}

// noSurf uses a customized CSRF cookie with the Secure, Path and HttpOnly
// attributes set.
func (app *application) noSurf(next http.Handler) http.Handler {
	csrfHandler := nosurf.New(next)
	csrfHandler.SetBaseCookie(http.Cookie{
		HttpOnly: true,
		Path:     "/",
		Secure:   true,
	})

	// nosurf checks that the Origin (or Referer) of a form submission matches
	// this site, so it needs to know whether the browser is using HTTPS.
	// With -tls=off the connection to us is plain HTTP; behind a proxy we
	// believe its X-Forwarded-Proto header.
	csrfHandler.SetIsTLSFunc(func(r *http.Request) bool {
		switch {
		case app.config.tlsMode != "off":
			return true
		case app.config.behindProxy:
			return r.Header.Get("X-Forwarded-Proto") == "https"
		default:
			return false
		}
	})

	return csrfHandler
}

// authenticate checks whether the authenticatedUserID in the session belongs
// to a user that still exists in the database and whose password hasn't
// changed since this session logged in. If so, the user's ID is recorded in
// the request context so downstream handlers don't need to query it again.
func (app *application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Retrieve the authenticatedUserID value from the session using the
		// GetInt() method. This will return the zero value for an int (0) if no
		// "authenticatedUserID" value is in the session -- in which case we
		// call the next handler in the chain as normal and return.
		id := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
		if id == 0 {
			next.ServeHTTP(w, r)
			return
		}

		// Otherwise, look up the user's current session version.
		version, err := app.users.SessionVersion(id)
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			app.serverError(w, r, err)
			return
		}

		// If the user no longer exists, or their password has been changed
		// since this session was created (so the versions differ), log the
		// session out and carry on as an anonymous request.
		if errors.Is(err, models.ErrNoRecord) || version != app.sessionManager.GetInt(r.Context(), "sessionVersion") {
			app.sessionManager.Remove(r.Context(), "authenticatedUserID")
			app.sessionManager.Remove(r.Context(), "sessionVersion")
			next.ServeHTTP(w, r)
			return
		}

		// We know the request is coming from an authenticated user who exists
		// in our database. We create a new copy of the request with their ID
		// in the request context and assign it to r.
		ctx := context.WithValue(r.Context(), authenticatedUserIDContextKey, id)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}
