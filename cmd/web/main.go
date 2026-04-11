package main

import (
	"database/sql"
	"flag"
	"html/template"
	"log/slog"
	"net/http"
	"os"

	"github.com/satyaorz/snippetbox/internal/models"

	_ "github.com/go-sql-driver/mysql"
)

// an application struct to hold the application-wide dependencies for the
// web application. For now we'll only include the stuctured logger, but we'll
// add mroe to this as the uild progresses.
type application struct {
	logger        *slog.Logger
	snippets      *models.SnippetModel
	templateCache map[string]*template.Template
}

func main() {
	// Define a new command-line flag with the name "addr", a default value of ":4000"
	addr := flag.String("addr", ":4000", "HTTP netword address")

	dsn := flag.String("dsn", "web:pass@/snippetbox?parseTime=true", "MySQL data source name")

	flag.Parse()

	// the slog.New() function to initialize a new stuctured logger, which
	// writes to the standard out stream and uses the default settings.
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	db, err := openDB(*dsn)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	defer db.Close()

	templateCache, err := newTemplateCache()
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	defer db.Close()

	// Initialize a new template cache
	if err != nil {
		logger.Error(err.Error())
	}

	// And add it to the application dependecies

	app := &application{
		logger:   logger,
		snippets: &models.SnippetModel{DB: db},
		templateCache: templateCache,
	}

	// the info() method t the log the staring server messgae at Info severity
	// (along with the listen address as an attribute)
	logger.Info("Starting server", slog.Any("addr", *addr))

	err = http.ListenAndServe(*addr, app.routes())
	logger.Error(err.Error())
	os.Exit(1)
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
