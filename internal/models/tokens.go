package models

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"errors"
	"strings"
	"time"
)

// TokenPrefix starts every API token, which makes them easy to recognise (for
// example by secret scanners) if one ends up somewhere it shouldn't.
const TokenPrefix = "sb_"

// TokenModelInterface describes the methods our handlers need from a token
// store, so that tests can swap in a mock implementation.
type TokenModelInterface interface {
	New(userID int, name string, ttl time.Duration) (string, time.Time, error)
	UserID(token string) (int, error)
	Delete(token string) error
	DeleteAllForUser(userID int) error
	DeleteExpired() (int64, error)
}

// TokenModel stores API bearer tokens.
type TokenModel struct {
	DB *sql.DB
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// New creates a token for the user that expires after ttl, and returns it in
// plain text along with its expiry. The plain-text token is never stored, so
// this is the only time it's available.
func (m *TokenModel) New(userID int, name string, ttl time.Duration) (string, time.Time, error) {
	// 20 random bytes (160 bits) encode to exactly 32 base32 characters.
	b := make([]byte, 20)
	_, err := rand.Read(b)
	if err != nil {
		return "", time.Time{}, err
	}
	token := TokenPrefix + strings.ToLower(base32.StdEncoding.EncodeToString(b))

	expires := time.Now().UTC().Add(ttl).Truncate(time.Second)

	stmt := `INSERT INTO tokens (hash, user_id, name, created, expires) VALUES (?, ?, ?, UTC_TIMESTAMP(), ?)`

	_, err = m.DB.Exec(stmt, hashToken(token), userID, name, expires)
	if err != nil {
		return "", time.Time{}, err
	}

	return token, expires, nil
}

// UserID returns the ID of the user a valid, unexpired token belongs to, or
// ErrInvalidCredentials.
func (m *TokenModel) UserID(token string) (int, error) {
	// Cheap format check first, so obviously bogus tokens don't hit the
	// database.
	if !strings.HasPrefix(token, TokenPrefix) || len(token) != len(TokenPrefix)+32 {
		return 0, ErrInvalidCredentials
	}

	var userID int

	stmt := `SELECT user_id FROM tokens WHERE hash = ? AND expires > UTC_TIMESTAMP()`

	err := m.DB.QueryRow(stmt, hashToken(token)).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrInvalidCredentials
		}
		return 0, err
	}

	return userID, nil
}

// Delete revokes a single token.
func (m *TokenModel) Delete(token string) error {
	_, err := m.DB.Exec(`DELETE FROM tokens WHERE hash = ?`, hashToken(token))
	return err
}

// DeleteAllForUser revokes every token belonging to the user, e.g. after
// their password changes.
func (m *TokenModel) DeleteAllForUser(userID int) error {
	_, err := m.DB.Exec(`DELETE FROM tokens WHERE user_id = ?`, userID)
	return err
}

// DeleteExpired removes expired tokens and returns how many were deleted.
func (m *TokenModel) DeleteExpired() (int64, error) {
	result, err := m.DB.Exec(`DELETE FROM tokens WHERE expires <= UTC_TIMESTAMP()`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
