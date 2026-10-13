package models

import (
	"strings"
	"testing"
	"time"

	"github.com/satyaorz/snippetbox/internal/assert"
)

func TestTokenModel(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	db := newTestDB(t)
	m := TokenModel{DB: db}

	token, expires, err := m.New(1, "laptop", time.Hour)
	assert.NilError(t, err)
	assert.Equal(t, strings.HasPrefix(token, TokenPrefix), true)
	assert.Equal(t, len(token), len(TokenPrefix)+32)
	assert.Equal(t, expires.After(time.Now()), true)

	id, err := m.UserID(token)
	assert.NilError(t, err)
	assert.Equal(t, id, 1)

	// Only the hash is stored, never the token itself.
	var stored int
	err = db.QueryRow("SELECT COUNT(*) FROM tokens WHERE hash = ?", token).Scan(&stored)
	assert.NilError(t, err)
	assert.Equal(t, stored, 0)

	_, err = m.UserID("sb_" + strings.Repeat("a", 32))
	assert.Equal(t, err, ErrInvalidCredentials)
	_, err = m.UserID("not-a-token")
	assert.Equal(t, err, ErrInvalidCredentials)

	assert.NilError(t, m.Delete(token))
	_, err = m.UserID(token)
	assert.Equal(t, err, ErrInvalidCredentials)
}

func TestTokenModelExpiryAndRevokeAll(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	db := newTestDB(t)
	m := TokenModel{DB: db}

	expired, _, err := m.New(1, "old", -time.Minute)
	assert.NilError(t, err)
	_, err = m.UserID(expired)
	assert.Equal(t, err, ErrInvalidCredentials)

	n, err := m.DeleteExpired()
	assert.NilError(t, err)
	assert.Equal(t, n, int64(1))

	alice1, _, _ := m.New(1, "a", time.Hour)
	alice2, _, _ := m.New(1, "b", time.Hour)
	bob, _, _ := m.New(2, "c", time.Hour)

	assert.NilError(t, m.DeleteAllForUser(1))

	_, err = m.UserID(alice1)
	assert.Equal(t, err, ErrInvalidCredentials)
	_, err = m.UserID(alice2)
	assert.Equal(t, err, ErrInvalidCredentials)

	id, err := m.UserID(bob)
	assert.NilError(t, err)
	assert.Equal(t, id, 2)
}
