package main

import (
	"net/http"

	"github.com/satyaorz/snippetbox/ui"

	"github.com/justinas/alice"
)

// The routes() method returns a http.Handler containing our application routes.
func (app *application) routes() http.Handler {
	mux := http.NewServeMux()

	// Serve static files from the embedded filesystem. Because the embedded
	// files live under "static/...", a request for /static/css/main.css maps
	// directly onto them without needing to strip a prefix.
	mux.Handle("GET /static/", http.FileServerFS(ui.Files))

	// The syntax highlighting stylesheet is generated from the chroma style,
	// so it's served by a handler rather than from ui/static. This pattern is
	// more specific than "GET /static/", so it takes precedence.
	mux.HandleFunc("GET /static/css/syntax.css", app.syntaxCSSHandler)

	// A simple health-check endpoint that doesn't need any middleware.
	mux.HandleFunc("GET /ping", ping)

	// Create a new middleware chain containing the middleware specific to our
	// dynamic application routes: session loading/saving, CSRF protection and
	// authentication. The static file route doesn't need any of these.
	dynamic := alice.New(app.sessionManager.LoadAndSave, app.noSurf, app.authenticate)

	// Unprotected application routes using the "dynamic" middleware chain.
	mux.Handle("GET /{$}", dynamic.ThenFunc(app.home))
	mux.Handle("GET /about", dynamic.ThenFunc(app.about))
	mux.Handle("GET /snippet/view/{slug}", dynamic.ThenFunc(app.snippetView))
	mux.Handle("GET /snippet/raw/{slug}", dynamic.ThenFunc(app.snippetRaw))
	mux.Handle("GET /user/signup", dynamic.ThenFunc(app.userSignup))
	mux.Handle("GET /user/login", dynamic.ThenFunc(app.userLogin))

	// Form submissions that check a password are rate limited per IP address
	// to slow down brute-force attacks.
	limited := dynamic.Append(app.limitAuthAttempts)

	mux.Handle("POST /user/signup", limited.ThenFunc(app.userSignupPost))
	mux.Handle("POST /user/login", limited.ThenFunc(app.userLoginPost))

	// Protected (authenticated-only) application routes, using a new "protected"
	// middleware chain which includes the requireAuthentication middleware.
	protected := dynamic.Append(app.requireAuthentication)

	mux.Handle("GET /snippet/create", protected.ThenFunc(app.snippetCreate))
	mux.Handle("POST /snippet/create", protected.ThenFunc(app.snippetCreatePost))
	mux.Handle("GET /snippet/edit/{slug}", protected.ThenFunc(app.snippetEdit))
	mux.Handle("POST /snippet/edit/{slug}", protected.ThenFunc(app.snippetEditPost))
	mux.Handle("POST /snippet/delete/{slug}", protected.ThenFunc(app.snippetDeletePost))
	mux.Handle("POST /user/logout", protected.ThenFunc(app.userLogoutPost))
	mux.Handle("GET /account/view", protected.ThenFunc(app.accountView))
	mux.Handle("GET /account/password/update", protected.ThenFunc(app.accountPasswordUpdate))
	mux.Handle("POST /account/password/update", protected.Append(app.limitAuthAttempts).ThenFunc(app.accountPasswordUpdatePost))

	// The "standard" middleware chain which is used for every request. The
	// request ID comes first so every log line can include it, and
	// recoverPanic sits inside logRequest so a panic is logged as a 500.
	standard := alice.New(app.requestID, app.logRequest, app.recoverPanic, commonHeaders)

	return standard.Then(mux)
}
