package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"expvar"
	"net/http"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"
)

// Request metrics, updated by the logRequest middleware and published at
// /debug/vars on the metrics server. expvar names are global to the process,
// so these are package-level and registered exactly once.
var (
	metricRequestsReceived  = expvar.NewInt("total_requests_received")
	metricResponsesSent     = expvar.NewInt("total_responses_sent")
	metricProcessingTimeUS  = expvar.NewInt("total_processing_time_us")
	metricResponsesByStatus = expvar.NewMap("total_responses_sent_by_status")
)

// publishRuntimeMetrics adds the build version, goroutine count and database
// connection pool stats to /debug/vars.
func publishRuntimeMetrics(db *sql.DB) {
	if expvar.Get("version") != nil {
		return
	}
	expvar.NewString("version").Set(version())
	expvar.Publish("goroutines", expvar.Func(func() any { return runtime.NumGoroutine() }))
	expvar.Publish("database", expvar.Func(func() any { return db.Stats() }))
	expvar.Publish("timestamp", expvar.Func(func() any { return time.Now().Unix() }))
}

// pinger is satisfied by *sql.DB; tests can pass a fake.
type pinger interface {
	PingContext(ctx context.Context) error
}

// metricsRoutes serves the private metrics and health check endpoints. They
// run on a separate address (-metrics-addr) so they're never exposed through
// the public server.
func metricsRoutes(db pinger) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /debug/vars", expvar.Handler())

	// /healthz reports whether the app can reach the database, for load
	// balancers and container orchestrators.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		status, body := http.StatusOK, map[string]string{"status": "ok", "version": version()}
		if err := db.PingContext(ctx); err != nil {
			status, body = http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "error": err.Error()}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(body)
	})

	return mux
}

// recordResponse updates the request metrics once a response has been sent.
func recordResponse(status int, duration time.Duration) {
	metricResponsesSent.Add(1)
	metricProcessingTimeUS.Add(duration.Microseconds())
	metricResponsesByStatus.Add(strconv.Itoa(status), 1)
}

// version returns the VCS revision the binary was built from (with a
// "-dirty" suffix if there were uncommitted changes), or "unknown".
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	var revision string
	var modified bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}

	if revision == "" {
		return "unknown"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}
