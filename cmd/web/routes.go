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

	// A simple health-check endpoint that doesn't need any middleware.
	mux.HandleFunc("GET /ping", ping)

	// Create a new middleware chain containing the middleware specific to our
	// dynamic application routes: session loading/saving, CSRF protection and
	// authentication. The static file route doesn't need any of these.
	dynamic := alice.New(app.sessionManager.LoadAndSave, noSurf, app.authenticate)

	// Unprotected application routes using the "dynamic" middleware chain.
	mux.Handle("GET /{$}", dynamic.ThenFunc(app.home))
	mux.Handle("GET /snippet/view/{id}", dynamic.ThenFunc(app.snippetView))
	mux.Handle("GET /user/signup", dynamic.ThenFunc(app.userSignup))
	mux.Handle("POST /user/signup", dynamic.ThenFunc(app.userSignupPost))
	mux.Handle("GET /user/login", dynamic.ThenFunc(app.userLogin))
	mux.Handle("POST /user/login", dynamic.ThenFunc(app.userLoginPost))

	// Protected (authenticated-only) application routes, using a new "protected"
	// middleware chain which includes the requireAuthentication middleware.
	protected := dynamic.Append(app.requireAuthentication)

	mux.Handle("GET /snippet/create", protected.ThenFunc(app.snippetCreate))
	mux.Handle("POST /snippet/create", protected.ThenFunc(app.snippetCreatePost))
	mux.Handle("POST /user/logout", protected.ThenFunc(app.userLogoutPost))

	// The "standard" middleware chain which is used for every request.
	standard := alice.New(app.recoverPanic, app.logRequest, commonHeaders)

	return standard.Then(mux)
}
