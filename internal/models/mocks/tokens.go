package mocks

import (
	"sync"
	"time"

	"github.com/satyaorz/snippetbox/internal/models"
)

// ValidToken is accepted by TokenModel as belonging to user 1 (Alice) until
// it's revoked.
const ValidToken = "sb_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// TokenModel is an in-memory stand-in for models.TokenModel.
type TokenModel struct {
	mu      sync.Mutex
	revoked bool
}

func (m *TokenModel) New(userID int, name string, ttl time.Duration) (string, time.Time, error) {
	return ValidToken, time.Now().Add(ttl), nil
}

func (m *TokenModel) UserID(token string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if token == ValidToken && !m.revoked {
		return 1, nil
	}
	return 0, models.ErrInvalidCredentials
}

func (m *TokenModel) Delete(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if token == ValidToken {
		m.revoked = true
	}
	return nil
}

func (m *TokenModel) DeleteAllForUser(userID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if userID == 1 {
		m.revoked = true
	}
	return nil
}

func (m *TokenModel) DeleteExpired() (int64, error) {
	return 0, nil
}
