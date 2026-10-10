package mocks

import (
	"github.com/satyaorz/snippetbox/internal/models"
)

// UserModel is an in-memory stand-in for models.UserModel.
//   - "dupe@example.com" is treated as already registered.
//   - "alice@example.com" / "pa$$word" are valid credentials for user ID 1.
type UserModel struct{}

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

func (m *UserModel) Exists(id int) (bool, error) {
	switch id {
	case 1:
		return true, nil
	default:
		return false, nil
	}
}
