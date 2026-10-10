package main

import (
	"crypto/tls"
	"database/sql"
	"flag"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/satyaorz/snippetbox/internal/models"

	"github.com/alexedwards/scs/mysqlstore"
	"github.com/alexedwards/scs/v2"
	"github.com/go-playground/form/v4"

	_ "github.com/go-sql-driver/mysql"
)

// an application struct to hold the application-wide dependencies for the
// web application. For now we'll only include the stuctured logger, but we'll
// add mroe to this as the uild progresses.
type application struct {
	debug          bool
	logger         *slog.Logger
	snippets       models.SnippetModelInterface
	users          models.UserModelInterface
	templateCache  map[string]*template.Template
	formDecoder    *form.Decoder
	sessionManager *scs.SessionManager
}

func main() {
	// Define a new command-line flag with the name "addr", a default value of ":4000"
	addr := flag.String("addr", ":4000", "HTTP network address")

	dsn := flag.String("dsn", "web:pass@/snippetbox?parseTime=true", "MySQL data source name")

	// Debug mode shows detailed errors and stack traces in HTTP responses.
	// Never enable it in production.
	debug := flag.Bool("debug", false, "Enable debug mode")

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

	formDecoder := form.NewDecoder()

	// Use the scs.New() function to initialize a new session manager. Then we
	// configure it to use our MySQL database as the session store, and set a
	// lifetime of 12 hours (so that sessions automatically expire 12 hours
	// after first being created).
	sessionManager := scs.New()
	sessionManager.Store = mysqlstore.New(db)
	sessionManager.Lifetime = 12 * time.Hour
	// Make sure that the Secure attribute is set on our session cookies, so
	// they are only ever sent over an HTTPS connection.
	sessionManager.Cookie.Secure = true

	app := &application{
		debug:          *debug,
		logger:         logger,
		snippets:       &models.SnippetModel{DB: db},
		users:          &models.UserModel{DB: db},
		templateCache:  templateCache,
		formDecoder:    formDecoder,
		sessionManager: sessionManager,
	}

	// the info() method t the log the staring server messgae at Info severity
	// (along with the listen address as an attribute)
	logger.Info("Starting server", slog.Any("addr", *addr))

	// Only use elliptic curves with assembly implementations, which perform
	// better under heavy load.
	tlsConfig := &tls.Config{
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
	}

	// Initialize a new http.Server struct so we can use our structured logger
	// for server errors and set sensible timeouts.
	srv := &http.Server{
		Addr:    *addr,
		Handler: app.routes(),
		// Route any log entries from the server through our structured logger
		// at Error level.
		ErrorLog:  slog.NewLogLogger(logger.Handler(), slog.LevelError),
		TLSConfig: tlsConfig,
		// Close keep-alive connections after one minute of inactivity, and
		// protect against slow-client attacks with read/write timeouts.
		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Use ListenAndServeTLS() to start the HTTPS server, passing in the paths
	// to the TLS certificate and corresponding private key.
	err = srv.ListenAndServeTLS("./tls/cert.pem", "./tls/key.pem")
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
