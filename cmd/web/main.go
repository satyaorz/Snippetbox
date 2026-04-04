package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
)

//an application struct to hold the application-wide dependencies for the
// web application. For now we'll only include the stuctured logger, but we'll 
// add mroe to this as the uild progresses.
type application struct {
	logger *slog.Logger
}

func main() {
	// Define a new command-line flag with the name "addr", a default value of ":4000"
	addr := flag.String("addr", ":4000", "HTTP netword address")

	flag.Parse()

	// the slog.New() function to initialize a new stuctured logger, which
	// writes to the standard out stream and uses the default settings.
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app := &application{
		logger : logger,
	}
	
	// the info() method t the log the staring server messgae at Info severity
	// (along with the listen address as an attribute)
	logger.Info("Starting server", slog.Any("addr", *addr))

	err := http.ListenAndServe(*addr, app.routes()) 
	logger.Error(err.Error())
	os.Exit(1)
}