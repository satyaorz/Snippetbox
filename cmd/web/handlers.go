package main

import (
	"fmt"
	"net/http"
	"strconv"
)

// Define a home handler function which writes a byte slice containing
// "Hello from Snippetbox" as the response body.
// w is used to send a response back to the client, and r contains all details about the incoming request
func home(w http.ResponseWriter, r *http.Request) {
	// SUe the Header().Add() method to add a 'Server: Go' header to the
	// response header map. The first paraeter is the header name, and
	// the second parameter is the header value.
	w.Header().Add("Server", "Go")

	fmt.Fprint(w, "Hello from Snippetbox\n")
}

func snippetView(w http.ResponseWriter, r *http.Request) {
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

func snippetCreate(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Display a form for creating a new snippet...\n"))
}

func snippetCreatePost(w http.ResponseWriter, r *http.Request) {
	// Use the w.WriteHeader() method to send a 201 status code.
	w.WriteHeader(http.StatusCreated)

	fmt.Fprint(w, "Save a new snippet...\n")
}