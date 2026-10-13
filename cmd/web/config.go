package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"
)

// config holds all the settings for the web server. Every setting is a
// command-line flag, and can also be set with an environment variable named
// after the flag: -cleanup-interval becomes SNIPPETBOX_CLEANUP_INTERVAL. A flag
// given on the command line always wins over the environment.
type config struct {
	addr            string
	dsn             string
	debug           bool
	cleanupInterval time.Duration
	shutdownTimeout time.Duration

	// TLS mode: "file" (certificate and key files), "autocert" (Let's
	// Encrypt) or "off" (plain HTTP, for use behind a TLS-terminating proxy).
	tlsMode          string
	tlsCert          string
	tlsKey           string
	autocertDomains  []string
	autocertEmail    string
	autocertCache    string
	autocertHTTPAddr string

	// behindProxy trusts the X-Forwarded-For, X-Forwarded-Proto and
	// X-Request-Id headers set by a reverse proxy in front of the app.
	behindProxy bool

	logFormat   string
	logLevel    slog.Level
	metricsAddr string
	version     bool
}

const envPrefix = "SNIPPETBOX_"

// envName returns the environment variable that can set the named flag.
func envName(flagName string) string {
	return envPrefix + strings.ToUpper(strings.ReplaceAll(flagName, "-", "_"))
}

// parseConfig reads the configuration from command-line args and the
// environment (via getenv, so tests can supply their own).
func parseConfig(args []string, getenv func(string) string, output io.Writer) (config, error) {
	var cfg config

	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(output)

	fs.StringVar(&cfg.addr, "addr", ":4000", "HTTP network address")
	fs.StringVar(&cfg.dsn, "dsn", "web:pass@/snippetbox?parseTime=true", "MySQL data source name")
	// Debug mode shows detailed errors and stack traces in HTTP responses.
	// Never enable it in production.
	fs.BoolVar(&cfg.debug, "debug", false, "Enable debug mode")
	fs.DurationVar(&cfg.cleanupInterval, "cleanup-interval", time.Hour, "How often to delete expired snippets")
	fs.DurationVar(&cfg.shutdownTimeout, "shutdown-timeout", 20*time.Second, "How long to wait for in-flight requests when shutting down")

	fs.StringVar(&cfg.tlsMode, "tls", "file", "TLS mode: file, autocert or off")
	fs.StringVar(&cfg.tlsCert, "tls-cert", "./tls/cert.pem", "TLS certificate file (-tls=file)")
	fs.StringVar(&cfg.tlsKey, "tls-key", "./tls/key.pem", "TLS private key file (-tls=file)")
	autocertDomains := fs.String("autocert-domains", "", "Comma-separated domains to get Let's Encrypt certificates for (-tls=autocert)")
	fs.StringVar(&cfg.autocertEmail, "autocert-email", "", "Contact email for Let's Encrypt (-tls=autocert)")
	fs.StringVar(&cfg.autocertCache, "autocert-cache", "./autocert-cache", "Directory to cache Let's Encrypt certificates in (-tls=autocert)")
	fs.StringVar(&cfg.autocertHTTPAddr, "autocert-http-addr", ":80", "Address for the ACME challenge / HTTPS redirect server (-tls=autocert)")

	fs.BoolVar(&cfg.behindProxy, "behind-proxy", false, "Trust X-Forwarded-For/-Proto and X-Request-Id headers from a reverse proxy")

	fs.StringVar(&cfg.logFormat, "log-format", "text", "Log format: text or json")
	logLevel := fs.String("log-level", "info", "Minimum log level: debug, info, warn or error")
	fs.StringVar(&cfg.metricsAddr, "metrics-addr", "", "Address for the private metrics and health check server (disabled if empty)")
	fs.BoolVar(&cfg.version, "version", false, "Print the version and exit")

	err := fs.Parse(args)
	if err != nil {
		return config{}, err
	}

	// Fill in any flag that wasn't given on the command line from its
	// environment variable. fs.Set() parses the value exactly as if it had
	// been passed as a flag, so "30s" or "true" work as expected.
	setOnCommandLine := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { setOnCommandLine[f.Name] = true })

	var envErrs []error
	fs.VisitAll(func(f *flag.Flag) {
		if setOnCommandLine[f.Name] {
			return
		}
		if value := getenv(envName(f.Name)); value != "" {
			if err := fs.Set(f.Name, value); err != nil {
				envErrs = append(envErrs, fmt.Errorf("%s: %w", envName(f.Name), err))
			}
		}
	})
	if err := errors.Join(envErrs...); err != nil {
		return config{}, err
	}

	for _, d := range strings.Split(*autocertDomains, ",") {
		if d = strings.TrimSpace(d); d != "" {
			cfg.autocertDomains = append(cfg.autocertDomains, d)
		}
	}

	err = cfg.logLevel.UnmarshalText([]byte(*logLevel))
	if err != nil {
		return config{}, fmt.Errorf("invalid log level %q", *logLevel)
	}

	return cfg, cfg.validate()
}

func (cfg config) validate() error {
	if !slices.Contains([]string{"file", "autocert", "off"}, cfg.tlsMode) {
		return fmt.Errorf("invalid TLS mode %q: must be file, autocert or off", cfg.tlsMode)
	}
	if cfg.tlsMode == "autocert" && len(cfg.autocertDomains) == 0 {
		return errors.New("-tls=autocert needs at least one domain in -autocert-domains")
	}
	if !slices.Contains([]string{"text", "json"}, cfg.logFormat) {
		return fmt.Errorf("invalid log format %q: must be text or json", cfg.logFormat)
	}
	if cfg.cleanupInterval <= 0 {
		return errors.New("-cleanup-interval must be positive")
	}
	return nil
}

// newLogger creates the structured logger described by the config.
func newLogger(cfg config, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.logLevel}

	if cfg.logFormat == "json" {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}

// defaultGetenv is os.Getenv; it's a variable so main stays tiny.
var defaultGetenv = os.Getenv
