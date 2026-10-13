package main

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/satyaorz/snippetbox/internal/assert"
)

func TestParseConfig(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(key string) string { return vars[key] }
	}

	t.Run("Defaults", func(t *testing.T) {
		cfg, err := parseConfig(nil, env(nil), io.Discard)
		assert.NilError(t, err)

		assert.Equal(t, cfg.addr, ":4000")
		assert.Equal(t, cfg.tlsMode, "file")
		assert.Equal(t, cfg.cleanupInterval, time.Hour)
		assert.Equal(t, cfg.logLevel, slog.LevelInfo)
		assert.Equal(t, cfg.behindProxy, false)
	})

	t.Run("Environment variables", func(t *testing.T) {
		cfg, err := parseConfig(nil, env(map[string]string{
			"SNIPPETBOX_ADDR":             ":8080",
			"SNIPPETBOX_TLS":              "off",
			"SNIPPETBOX_BEHIND_PROXY":     "true",
			"SNIPPETBOX_CLEANUP_INTERVAL": "5m",
			"SNIPPETBOX_LOG_FORMAT":       "json",
			"SNIPPETBOX_LOG_LEVEL":        "debug",
		}), io.Discard)
		assert.NilError(t, err)

		assert.Equal(t, cfg.addr, ":8080")
		assert.Equal(t, cfg.tlsMode, "off")
		assert.Equal(t, cfg.behindProxy, true)
		assert.Equal(t, cfg.cleanupInterval, 5*time.Minute)
		assert.Equal(t, cfg.logFormat, "json")
		assert.Equal(t, cfg.logLevel, slog.LevelDebug)
	})

	t.Run("Flags beat environment variables", func(t *testing.T) {
		cfg, err := parseConfig([]string{"-addr", ":9000"}, env(map[string]string{
			"SNIPPETBOX_ADDR": ":8080",
		}), io.Discard)
		assert.NilError(t, err)
		assert.Equal(t, cfg.addr, ":9000")
	})

	t.Run("Autocert domains", func(t *testing.T) {
		cfg, err := parseConfig([]string{"-tls", "autocert", "-autocert-domains", "a.example.com, b.example.com"}, env(nil), io.Discard)
		assert.NilError(t, err)
		assert.Equal(t, len(cfg.autocertDomains), 2)
		assert.Equal(t, cfg.autocertDomains[1], "b.example.com")
	})

	errorTests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{name: "Bad TLS mode", args: []string{"-tls", "maybe"}},
		{name: "Autocert without domains", args: []string{"-tls", "autocert"}},
		{name: "Bad log format", args: []string{"-log-format", "xml"}},
		{name: "Bad log level", args: []string{"-log-level", "loud"}},
		{name: "Bad env duration", env: map[string]string{"SNIPPETBOX_CLEANUP_INTERVAL": "soon"}},
		{name: "Bad env bool", env: map[string]string{"SNIPPETBOX_DEBUG": "perhaps"}},
		{name: "Unknown flag", args: []string{"-nope"}},
	}

	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseConfig(tt.args, env(tt.env), io.Discard)
			if err == nil {
				t.Error("expected an error")
			}
		})
	}
}
