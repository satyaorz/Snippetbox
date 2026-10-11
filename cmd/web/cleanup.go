package main

import (
	"context"
	"time"
)

// cleanupExpiredSnippets deletes expired snippets once immediately and then
// every interval, until ctx is cancelled. Expired snippets are already hidden
// by every query; this just stops them piling up in the database.
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

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
