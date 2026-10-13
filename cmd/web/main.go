package main

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/satyaorz/snippetbox/internal/models"

	"github.com/alexedwards/scs/mysqlstore"
	"github.com/alexedwards/scs/v2"
	"github.com/go-playground/form/v4"
	"golang.org/x/time/rate"

	_ "github.com/go-sql-driver/mysql"
)

// application holds the application-wide dependencies for the web
// application, so handlers and middleware can use them as methods.
type application struct {
	config         config
	logger         *slog.Logger
	snippets       models.SnippetModelInterface
	users          models.UserModelInterface
	templateCache  map[string]*template.Template
	formDecoder    *form.Decoder
	sessionManager *scs.SessionManager
	authLimiter    *ipRateLimiter
}

func main() {
	cfg, err := parseConfig(os.Args[1:], defaultGetenv, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if cfg.version {
		fmt.Println(version())
		return
	}

	logger := newLogger(cfg, os.Stdout)

	err = run(cfg, logger)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}

// run wires up the application and serves it until it receives SIGINT or
// SIGTERM, then shuts down gracefully.
func run(cfg config, logger *slog.Logger) error {
	db, err := openDB(cfg.dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	templateCache, err := newTemplateCache()
	if err != nil {
		return err
	}

	// Use the scs.New() function to initialize a new session manager. Then we
	// configure it to use our MySQL database as the session store, and set a
	// lifetime of 12 hours (so that sessions automatically expire 12 hours
	// after first being created).
	sessionStore := mysqlstore.New(db)
	defer sessionStore.StopCleanup()

	sessionManager := scs.New()
	sessionManager.Store = sessionStore
	sessionManager.Lifetime = 12 * time.Hour
	// Make sure that the Secure attribute is set on our session cookies, so
	// they are only ever sent over an HTTPS connection. (Behind a proxy the
	// browser still talks HTTPS to the proxy, so this stays on.)
	sessionManager.Cookie.Secure = true

	app := &application{
		config:         cfg,
		logger:         logger,
		snippets:       &models.SnippetModel{DB: db},
		users:          &models.UserModel{DB: db},
		templateCache:  templateCache,
		formDecoder:    form.NewDecoder(),
		sessionManager: sessionManager,
		// Allow each IP address a burst of 5 login/signup/password attempts,
		// then one more every 12 seconds (5 per minute).
		authLimiter: newIPRateLimiter(rate.Every(12*time.Second), 5),
	}

	if cfg.tlsMode == "off" && !cfg.behindProxy {
		logger.Warn("serving plain HTTP without -behind-proxy: secure session cookies only work on localhost")
	}

	publishRuntimeMetrics(db)

	// ctx is cancelled when the process receives SIGINT (Ctrl+C) or SIGTERM
	// (e.g. `docker stop`), which starts the graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Periodically delete expired snippets in the background until shutdown.
	var wg sync.WaitGroup
	wg.Go(func() { app.cleanupExpiredSnippets(ctx, cfg.cleanupInterval) })

	err = app.runServers(ctx, cfg.shutdownTimeout, app.buildServers(cfg, metricsRoutes(db))...)

	// If a server failed (rather than us being signalled), stop the background
	// job too, and wait for it before closing the database.
	stop()
	wg.Wait()

	return err
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
