package main

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
)

// *application
func (app *application) home(w http.ResponseWriter, r *http.Request) {
	// Use the Header().Add() method to add a 'Server: Go' header to the
	// response header map. The first paraeter is the header name, and
	// the second parameter is the header value.
	w.Header().Add("Server", "Go")

	// Initialize a slice containing the paths to the two files. It's important
	// to note that the files containg our base template mus be the *first*
	// file in the slice
	files := []string {
		"./ui/html/base.tmpl.html",
		"./ui/html/partials/nav.tmpl.html",
		"./ui/html/pages/home.tmpl.html",
	}

	// Use the template.ParseFiles() functin to read the files and store the
	// templates in a template set. Notice that we use .. to pass the contents
	// of the files slice as variadic arguments.
	// user, and then return from the handler so no susequent code is eecuted.
	ts, err := template.ParseFiles(files...)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Use the ExecuteTemplate() method to write the contents of the "base"
	// template as the response body.
	err = ts.ExecuteTemplate(w, "base", nil)
	if err != nil {
		app.serverError(w, r, err)
	}

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

	// reponse using fmt.Fprintf() instead of w.Write() as they have same interface
	fmt.Fprintf(w, "Display a specific snippet with Id %d...\n", id)
}

func (app *application) snippetCreate(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Display a form for creating a new snippet...\n"))
}

func (app *application) snippetCreatePost(w http.ResponseWriter, r *http.Request) {
	// Use the w.WriteHeader() method to send a 201 status code.
	w.WriteHeader(http.StatusCreated)

	w.Write([]byte("Save a new snippet...\n"))
}		