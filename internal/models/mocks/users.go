package mocks

import (
	"sync"
	"time"

	"github.com/satyaorz/snippetbox/internal/models"
)

// UserModel is an in-memory stand-in for models.UserModel.
//   - "dupe@example.com" is treated as already registered.
//   - "alice@example.com" / "pa$$word" are valid credentials for user ID 1.
//   - PasswordUpdate succeeds for user ID 1 when the current password is
//     "pa$$word", and bumps the session version just like the real model.
type UserModel struct {
	mu sync.Mutex
	// versionBumps counts successful password changes; the session version
	// is 1 + versionBumps.
	versionBumps int
}

func (m *UserModel) Insert(name, email, password string) error {
	switch email {
	case "dupe@example.com":
		return models.ErrDuplicateEmail
	default:
		return nil
	}
}

func (m *UserModel) Authenticate(email, password string) (int, error) {
	if email == "alice@example.com" && password == "pa$$word" {
		return 1, nil
	}

	return 0, models.ErrInvalidCredentials
}

func (m *UserModel) SessionVersion(id int) (int, error) {
	if id != 1 {
		return 0, models.ErrNoRecord
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return 1 + m.versionBumps, nil
}

func (m *UserModel) Get(id int) (models.User, error) {
	if id == 1 {
		return models.User{
			ID:      1,
			Name:    "Alice",
			Email:   "alice@example.com",
			Created: time.Date(2022, 1, 1, 9, 18, 24, 0, time.UTC),
		}, nil
	}

	return models.User{}, models.ErrNoRecord
}

func (m *UserModel) PasswordUpdate(id int, currentPassword, newPassword string) error {
	if id != 1 {
		return models.ErrNoRecord
	}
	if currentPassword != "pa$$word" {
		return models.ErrInvalidCredentials
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.versionBumps++
	return nil
}
