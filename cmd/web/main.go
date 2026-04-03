package main

import (
	"log"
	"net/http"
)


func main() {
	// Use the http.NewServeMux() function to intialize a new sermux
	// then reister the home function as the handle for the "/" URL pattern
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", home)
	mux.HandleFunc("GET /snippet/view/{id}", snippetView)
	mux.HandleFunc("GET /snippet/create", snippetCreate)

	// post
	mux.HandleFunc("POST /snippet/create", snippetCreatePost)

	// Print a log message to say that the server is starting.
	log.Print("starting server on: 4000")

	/*
	Use the http.ListenAnsServe() function to start a new web server. We pass in 
	two parameters: the TCP network address to listen on (in this case ":4000")
	and the servemux we just created. If http.ListenAndServe() returns an error we use the log.Fatal() function
	to log the error mesage and exit.
	Note that any error returned by http.ListenAndServe() is non-nill. 
	*/

	err := http.ListenAndServe(":4000", mux) // should be in format "host:port"
	log.Fatal(err)
}