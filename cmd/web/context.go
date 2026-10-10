package main

// Use a custom type for request context keys to avoid collisions with keys
// set by third-party packages.
type contextKey string

const isAuthenticatedContextKey = contextKey("isAuthenticated")
