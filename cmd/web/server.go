package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

// server is one HTTP server the app runs, along with how to start it.
type server struct {
	name  string
	srv   *http.Server
	start func() error
}

// buildServers creates the main application server for the configured TLS
// mode, plus the ACME challenge server (autocert mode) and the private
// metrics server (if -metrics-addr is set).
func (app *application) buildServers(cfg config, metrics http.Handler) []server {
	// Only use elliptic curves with assembly implementations, which perform
	// better under heavy load.
	tlsConfig := &tls.Config{
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
	}

	// Initialize a new http.Server struct so we can use our structured logger
	// for server errors and set sensible timeouts.
	main := &http.Server{
		Addr:    cfg.addr,
		Handler: app.routes(),
		// Route any log entries from the server through our structured logger
		// at Error level.
		ErrorLog: slog.NewLogLogger(app.logger.Handler(), slog.LevelError),
		// Close keep-alive connections after one minute of inactivity, and
		// protect against slow-client attacks with read/write timeouts.
		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	var servers []server

	switch cfg.tlsMode {
	case "file":
		main.TLSConfig = tlsConfig
		servers = append(servers, server{"app", main, func() error {
			return main.ListenAndServeTLS(cfg.tlsCert, cfg.tlsKey)
		}})

	case "autocert":
		// autocert gets (and renews) certificates from Let's Encrypt on
		// demand, for the listed domains only, caching them on disk.
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(cfg.autocertDomains...),
			Cache:      autocert.DirCache(cfg.autocertCache),
			Email:      cfg.autocertEmail,
		}
		main.TLSConfig = m.TLSConfig()
		main.TLSConfig.CurvePreferences = tlsConfig.CurvePreferences
		servers = append(servers, server{"app", main, func() error {
			return main.ListenAndServeTLS("", "")
		}})

		// Let's Encrypt's HTTP-01 challenge needs a plain HTTP server; any
		// other request to it is redirected to HTTPS.
		challenge := &http.Server{
			Addr:         cfg.autocertHTTPAddr,
			Handler:      m.HTTPHandler(nil),
			ErrorLog:     main.ErrorLog,
			IdleTimeout:  time.Minute,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		}
		servers = append(servers, server{"acme", challenge, challenge.ListenAndServe})

	case "off":
		servers = append(servers, server{"app", main, main.ListenAndServe})
	}

	if cfg.metricsAddr != "" {
		ms := &http.Server{
			Addr:         cfg.metricsAddr,
			Handler:      metrics,
			ErrorLog:     main.ErrorLog,
			IdleTimeout:  time.Minute,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		servers = append(servers, server{"metrics", ms, ms.ListenAndServe})
	}

	return servers
}

// runServers starts all the servers and blocks until ctx is cancelled (on
// SIGINT/SIGTERM) or one of them fails. It then shuts every server down
// gracefully: they stop accepting new connections and in-flight requests get
// up to shutdownTimeout to finish.
func (app *application) runServers(ctx context.Context, shutdownTimeout time.Duration, servers ...server) error {
	errs := make(chan error, len(servers))

	for _, s := range servers {
		go func() {
			app.logger.Info("starting server", "server", s.name, "addr", s.srv.Addr)

			// Shutdown() makes the start function return http.ErrServerClosed
			// straight away, which isn't an error for us.
			err := s.start()
			if !errors.Is(err, http.ErrServerClosed) {
				errs <- fmt.Errorf("%s server: %w", s.name, err)
			}
		}()
	}

	var runErr error
	select {
	case <-ctx.Done():
		app.logger.Info("shutting down servers", "timeout", shutdownTimeout.String())
	case runErr = <-errs:
		app.logger.Error("server failed, shutting down", "error", runErr)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	shutdownErrs := []error{runErr}
	for _, s := range servers {
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			shutdownErrs = append(shutdownErrs, fmt.Errorf("shutting down %s server: %w", s.name, err))
		}
	}

	err := errors.Join(shutdownErrs...)
	if err == nil {
		app.logger.Info("servers stopped")
	}
	return err
}
