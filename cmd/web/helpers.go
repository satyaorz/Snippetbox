package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	// "time"

	"github.com/go-playground/form/v4"
)

// The serverErro helper writes a log entry at Error level (including the request
// method and URI as attributes), then sends a generic 500 Internal Server Error
// response to the user.
func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		method = r.Method
		uri    = r.URL.RequestURI()
	)

	app.logger.Error(err.Error(), "Method", method, "uri", uri)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (app *application) render(w http.ResponseWriter, r *http.Request, status int, page string, data templateData) {
	ts, ok := app.templateCache[page]

	if !ok {
		err := fmt.Errorf("the themplate %s does not exist", page)
		app.serverError(w, r, err)
		return
	}

	buf := new(bytes.Buffer)
	
	err := ts.ExecuteTemplate(buf, "base", data)
	if err != nil {
		app.serverError(w, r, err)
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