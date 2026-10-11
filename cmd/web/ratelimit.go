package main

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ipRateLimiter keeps a token-bucket rate limiter per client IP address. It's
// used to slow down password guessing on the login, signup and change
// password forms.
type ipRateLimiter struct {
	mu        sync.Mutex
	clients   map[string]*rateLimitClient
	limit     rate.Limit
	burst     int
	lastSweep time.Time
}

type rateLimitClient struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Clients that haven't made a request for this long are forgotten. By then
// their bucket would have refilled anyway, so nothing is lost.
const rateLimitIdleTimeout = 10 * time.Minute

// newIPRateLimiter allows each IP address `burst` requests at once, refilled
// at `limit` requests per second.
func newIPRateLimiter(limit rate.Limit, burst int) *ipRateLimiter {
	return &ipRateLimiter{
		clients:   make(map[string]*rateLimitClient),
		limit:     limit,
		burst:     burst,
		lastSweep: time.Now(),
	}
}

// allow reports whether a request from ip may proceed right now.
func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// Rather than running a background goroutine, sweep out idle clients at
	// most once a minute as part of a normal request.
	if now.Sub(l.lastSweep) > time.Minute {
		for key, c := range l.clients {
			if now.Sub(c.lastSeen) > rateLimitIdleTimeout {
				delete(l.clients, key)
			}
		}
		l.lastSweep = now
	}

	c, ok := l.clients[ip]
	if !ok {
		c = &rateLimitClient{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.clients[ip] = c
	}
	c.lastSeen = now

	return c.limiter.Allow()
}

// retryAfter is roughly how many seconds a client must wait for a new token.
func (l *ipRateLimiter) retryAfter() string {
	if l.limit <= 0 || l.limit == rate.Inf {
		return "1"
	}
	return strconv.Itoa(max(1, int(1/float64(l.limit))))
}

// limitAuthAttempts rejects requests with 429 Too Many Requests once a client
// IP has used up its allowance. Note that r.RemoteAddr is the address of the
// direct peer: behind a reverse proxy every request would share one bucket.
func (app *application) limitAuthAttempts(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		if !app.authLimiter.allow(ip) {
			w.Header().Set("Retry-After", app.authLimiter.retryAfter())
			app.clientError(w, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
