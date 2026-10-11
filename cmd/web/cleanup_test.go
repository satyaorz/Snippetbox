package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/satyaorz/snippetbox/internal/assert"
	"github.com/satyaorz/snippetbox/internal/models/mocks"
)

// countingSnippetModel records how many times DeleteExpired is called.
type countingSnippetModel struct {
	mocks.SnippetModel
	calls atomic.Int64
}

func (m *countingSnippetModel) DeleteExpired() (int64, error) {
	m.calls.Add(1)
	return 2, nil
}

func TestCleanupExpiredSnippets(t *testing.T) {
	app := newTestApplication(t)
	model := &countingSnippetModel{}
	app.snippets = model

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		app.cleanupExpiredSnippets(ctx, 10*time.Millisecond)
		close(done)
	}()

	// It runs once straight away and then on every tick.
	deadline := time.Now().Add(2 * time.Second)
	for model.calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not stop after the context was cancelled")
	}

	assert.Equal(t, model.calls.Load() >= 3, true)
}
