package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/satyaorz/snippetbox/internal/assert"
)

func TestRunServersGracefulShutdown(t *testing.T) {
	app := newTestApplication(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	// A handler that takes a while, so we can shut down mid-request.
	started := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte("finished"))
	})}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- app.runServers(ctx, 5*time.Second, server{"test", srv, func() error { return srv.Serve(ln) }})
	}()

	type result struct {
		body string
		err  error
	}
	res := make(chan result)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err != nil {
			res <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		res <- result{string(b), err}
	}()

	// Start shutting down while the request is still being handled.
	<-started
	cancel()

	r := <-res
	assert.NilError(t, r.err)
	assert.Equal(t, r.body, "finished")
	assert.NilError(t, <-done)

	// New connections are refused once we've shut down.
	_, err = http.Get("http://" + ln.Addr().String())
	if err == nil {
		t.Error("expected the server to be closed")
	}
}

func TestRunServersFailure(t *testing.T) {
	app := newTestApplication(t)
	srv := &http.Server{}

	err := app.runServers(context.Background(), time.Second, server{"broken", srv, func() error {
		return errors.New("address already in use")
	}})

	assert.StringContains(t, err.Error(), "broken server: address already in use")
}

type fakePinger struct{ err error }

func (p fakePinger) PingContext(ctx context.Context) error { return p.err }

func TestHealthz(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantCode   int
		wantStatus string
	}{
		{name: "Healthy", wantCode: http.StatusOK, wantStatus: "ok"},
		{name: "Database down", pingErr: errors.New("connection refused"), wantCode: http.StatusServiceUnavailable, wantStatus: "unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/healthz", nil)

			metricsRoutes(fakePinger{tt.pingErr}).ServeHTTP(rr, r)

			assert.Equal(t, rr.Code, tt.wantCode)

			var body map[string]string
			assert.NilError(t, json.NewDecoder(rr.Body).Decode(&body))
			assert.Equal(t, body["status"], tt.wantStatus)
		})
	}
}

func TestMetricsVars(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	ts.get(t, "/ping")

	rr := httptest.NewRecorder()
	metricsRoutes(fakePinger{}).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/debug/vars", nil))

	assert.Equal(t, rr.Code, http.StatusOK)
	assert.StringContains(t, rr.Body.String(), `"total_requests_received"`)
	assert.StringContains(t, rr.Body.String(), `"total_responses_sent_by_status": {"200"`)
}

func TestRequestLogging(t *testing.T) {
	var buf bytes.Buffer
	app := newTestApplication(t)
	app.logger = slog.New(slog.NewJSONHandler(&buf, nil))
	ts := newTestServer(t, app.routes())

	res := ts.get(t, "/snippet/view/ffffffffffff")
	id := res.headers.Get("X-Request-Id")
	assert.Equal(t, len(id), 16)

	var entry map[string]any
	assert.NilError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &entry))
	assert.Equal[any](t, entry["msg"], "request")
	assert.Equal[any](t, entry["request_id"], id)
	assert.Equal[any](t, entry["status"], float64(http.StatusNotFound))
	assert.Equal[any](t, entry["uri"], "/snippet/view/ffffffffffff")
}
