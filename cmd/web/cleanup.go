package main

import (
	"context"
	"time"
)

// cleanupExpiredSnippets deletes expired snippets (and expired API tokens)
// once immediately and then every interval, until ctx is cancelled. Expired
// rows are already ignored by every query; this just stops them piling up in
// the database.
func (app *application) cleanupExpiredSnippets(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		n, err := app.snippets.DeleteExpired()
		if err != nil {
			app.logger.Error("deleting expired snippets", "error", err)
		} else if n > 0 {
			app.logger.Info("deleted expired snippets", "count", n)
		}

		n, err = app.tokens.DeleteExpired()
		if err != nil {
			app.logger.Error("deleting expired API tokens", "error", err)
		} else if n > 0 {
			app.logger.Info("deleted expired API tokens", "count", n)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
