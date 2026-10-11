package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/satyaorz/snippetbox/internal/models"

	"github.com/go-playground/form/v4"
	"github.com/justinas/nosurf"
)

// The serverError helper writes a log entry at Error level (including the
// request method, URI and a stack trace as attributes), then sends a generic
// 500 Internal Server Error response to the user. In debug mode the error and
// stack trace are included in the response body instead.
func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		method = r.Method
		uri    = r.URL.RequestURI()
		trace  = string(debug.Stack())
	)

	app.logger.Error(err.Error(), "method", method, "uri", uri, "trace", trace)

	if app.debug {
		body := fmt.Sprintf("%s\n%s", err, trace)
		http.Error(w, body, http.StatusInternalServerError)
		return
	}

	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (app *application) render(w http.ResponseWriter, r *http.Request, status int, page string, data templateData) {
	ts, ok := app.templateCache[page]

	if !ok {
		err := fmt.Errorf("the template %s does not exist", page)
		app.serverError(w, r, err)
		return
	}

	buf := new(bytes.Buffer)

	err := ts.ExecuteTemplate(buf, "base", data)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(status)
	buf.WriteTo(w)
}

func (app *application) decodePostForm(r *http.Request, dst any) error {
	// Call ParseFrom() on the request, int the same way that is done in snippetCreatePost handler.
	err := r.ParseForm()
	if err != nil {
		return err
	}

	err = app.formDecoder.Decode(dst, r.PostForm)
	if err != nil {
		var invalidDecoderError *form.InvalidDecoderError

		if errors.As(err, &invalidDecoderError) {
			panic(err)
		}
	}

	return err
}

// The clientError helper sends a specific status code and coreesponding description
// to the user. we'll use this later in the book to send responses like 400 "Bad Request"
// when there's a problem with the requeest that the user sent.

func (app *application) clientError(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

// newTemplateData returns an initialized templateData struct for use in
// rendering templates. It centralizes any common data we want available to
// all templates.
func (app *application) newTemplateData(r *http.Request) templateData {
	return templateData{
		CurrentYear: time.Now().Year(),
		// Retrieve the "flash" value from the session (and remove it) so it
		// is only ever shown once.
		Flash:               app.sessionManager.PopString(r.Context(), "flash"),
		IsAuthenticated:     app.isAuthenticated(r),
		AuthenticatedUserID: app.authenticatedUserID(r),
		CSRFToken:           nosurf.Token(r),
	}
}

// authenticatedUserID returns the ID of the logged-in user making the
// request, or 0 if the request is unauthenticated. The value is set by the
// authenticate middleware; if it's missing (or isn't an int), the type
// assertion fails and we treat the request as unauthenticated.
func (app *application) authenticatedUserID(r *http.Request) int {
	id, ok := r.Context().Value(authenticatedUserIDContextKey).(int)
	if !ok {
		return 0
	}

	return id
}

// isAuthenticated returns true if the current request is from an
// authenticated user, otherwise false.
func (app *application) isAuthenticated(r *http.Request) bool {
	return app.authenticatedUserID(r) != 0
}

// isOwner reports whether the current user owns the snippet. Anonymous
// snippets (UserID 0) have no owner.
func (app *application) isOwner(r *http.Request, snippet models.Snippet) bool {
	return snippet.UserID != 0 && snippet.UserID == app.authenticatedUserID(r)
}
