package main

import (
	"errors"
	"fmt"
	// "html/template"
	"net/http"
	"strconv"

	"github.com/satyaorz/snippetbox/internal/models"
)

// *application
func (app *application) home(w http.ResponseWriter, r *http.Request) {
	// Use the Header().Add() method to add a 'Server: Go' header to the
	// response header map. The first paraeter is the header name, and
	// the second parameter is the header value.
	w.Header().Add("Server", "Go")

	snippets, err := app.snippets.Latest()
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	for _, snippet := range snippets {
		fmt.Fprintf(w, "%+v\n", snippet)
	}

	// // Initialize a slice containing the paths to the two files. It's important
	// // to note that the files containg our base template mus be the *first*
	// // file in the slice
	// files := []string {
	// 	"./ui/html/base.tmpl.html",
	// 	"./ui/html/partials/nav.tmpl.html",
	// 	"./ui/html/pages/home.tmpl.html",
	// }

	// // Use the template.ParseFiles() functin to read the files and store the
	// // templates in a template set. Notice that we use .. to pass the contents
	// // of the files slice as variadic arguments.
	// // user, and then return from the handler so no susequent code is eecuted.
	// ts, err := template.ParseFiles(files...)
	// if err != nil {
	// 	app.serverError(w, r, err)
	// 	return
	// }

	// // Use the ExecuteTemplate() method to write the contents of the "base"
	// // template as the response body.
	// err = ts.ExecuteTemplate(w, "base", nil)
	// if err != nil {
	// 	app.serverError(w, r, err)
	// }

}

func (app *application) snippetView(w http.ResponseWriter, r *http.Request) {
	// Extract the value of the `id` wildcard from the reques using r.PathValue()
	// and try to convert it to n intefer using the strconv.Atoi() function. If
	// it can't be converted to an integer, or the value is less then 1, we return
	// a 404 page not found response.

	id, err := strconv.Atoi(r.PathValue("id"))

	if err != nil || id < 1 {
		http.NotFound(w, r)
		return 
	}

	// Use the SnippetModel's Get() method to retrieve the data for a 
	// specific record based on its ID. if no matching record is found,
	// return a 404 Not Found response.
	snippet, err := app.snippets.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			http.NotFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	// write the snippet data as a plain-text HTTP response body
	fmt.Fprintf(w, "%+v", snippet)
}

func (app *application) snippetCreate(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Display a form for creating a new snippet...\n"))
}

func (app *application) snippetCreatePost(w http.ResponseWriter, r *http.Request) {
	title := "0 snail"
	content := "0 snail\nClimb Mount Fuji, \nBut slowly, slowly!\n\n- kobayashi Issa"
	expires := 7

	// Pass the data to the snippetModel.Insert() method, receiving the Id the new record back.
	id, err := app.snippets.Insert(title, content, expires)
	if err != nil {
		app.serverError(w,r, err)
		return
	}
	
	// Redirect the user to the relevant page for the snippet.
	http.Redirect(w, r, fmt.Sprintf("/snippet/view/%d", id), http.StatusSeeOther)
}		