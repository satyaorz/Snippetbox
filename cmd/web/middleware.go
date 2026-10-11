package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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

func (app *application) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			ip     = r.RemoteAddr
			proto  = r.Proto
			method = r.Method
			uri    = r.URL.RequestURI()
		)

		app.logger.Info("received request", "ip", ip, "proto", proto, "method", method, "uri", uri)

		next.ServeHTTP(w, r)
	})
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
func noSurf(next http.Handler) http.Handler {
	csrfHandler := nosurf.New(next)
	csrfHandler.SetBaseCookie(http.Cookie{
		HttpOnly: true,
		Path:     "/",
		Secure:   true,
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
