package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
)


func main() {
	// Define a new command-line flag with the name "addr", a default value of ":4000"
	addr := flag.String("addr", ":4000", "HTTP netword address")

	flag.Parse()

	// the slog.New() function to initialize a new stuctured logger, which
	// writes to the standard out stream and uses the default settings.
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	mux := http.NewServeMux()

	fileServer := http.FileServer(http.Dir("./ui/static/"))

	mux.Handle("GET /static/", http.StripPrefix("/static", fileServer))

	mux.HandleFunc("GET /{$}", home)
	mux.HandleFunc("GET /snippet/view/{id}", snippetView)
	mux.HandleFunc("GET /snippet/create", snippetCreate)
	// post
	mux.HandleFunc("POST /snippet/create", snippetCreatePost)

	// the info() method t the log the staring server messgae at Info severity
	// (along with the listen address as an attribute)
	logger.Info("Starting server", "addr", *addr)

	/*
	Use the http.ListenAnsServe() function to start a new web server. We pass in 
	two parameters: the TCP network address to listen on (in this case ":4000")
	and the servemux we just created. If http.ListenAndServe() returns an error we use the log.Fatal() function
	to log the error mesage and exit.
	Note that any error returned by http.ListenAndServe() is non-nill. 
	*/

	err := http.ListenAndServe(*addr, mux) // should be in format "host:port"
	// And we also use the Error() method to log any error message returned by
	// http.ListenAndServe() at Error severity (with no additional attributes),
	// and then call os.Exit(1) to terminalte the application with exit code 1.
	logger.Error(err.Error())
	os.Exit(1)
}