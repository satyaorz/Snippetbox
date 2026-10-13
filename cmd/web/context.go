package main

// Use a custom type for request context keys to avoid collisions with keys
// set by third-party packages.
type contextKey string

const (
	// authenticatedUserIDContextKey holds the ID (an int) of the logged-in
	// user. It's only set by the authenticate middleware once the session has
	// been checked against the database.
	authenticatedUserIDContextKey = contextKey("authenticatedUserID")

	// requestIDContextKey holds the request's ID (a string), set by the
	// requestID middleware and included in log entries.
	requestIDContextKey = contextKey("requestID")
)
