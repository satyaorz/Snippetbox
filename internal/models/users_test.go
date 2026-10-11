package models

import (
	"testing"

	"github.com/satyaorz/snippetbox/internal/assert"

	_ "github.com/go-sql-driver/mysql"
)

// The seed data (testdata/setup.sql) contains two users, Alice (ID 1) and
// Bob (ID 2), who both have the password "pa$$word".

func TestUserModelSessionVersion(t *testing.T) {
	// Integration tests need a real MySQL database, so skip them when running
	// with the -short flag.
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	tests := []struct {
		name    string
		userID  int
		want    int
		wantErr error
	}{
		{name: "Valid ID", userID: 1, want: 1},
		{name: "Zero ID", userID: 0, wantErr: ErrNoRecord},
		{name: "Non-existent ID", userID: 99, wantErr: ErrNoRecord},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			m := UserModel{DB: db}

			version, err := m.SessionVersion(tt.userID)

			assert.Equal(t, version, tt.want)
			assert.Equal(t, err, tt.wantErr)
		})
	}
}

func TestUserModelInsertAndAuthenticate(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	db := newTestDB(t)
	m := UserModel{DB: db}

	err := m.Insert("Carol", "carol@example.com", "pa55word123")
	assert.NilError(t, err)

	err = m.Insert("Alice again", "alice@example.com", "whatever123")
	assert.Equal(t, err, ErrDuplicateEmail)

	id, err := m.Authenticate("carol@example.com", "pa55word123")
	assert.NilError(t, err)
	assert.Equal(t, id, 3)

	_, err = m.Authenticate("carol@example.com", "wrong")
	assert.Equal(t, err, ErrInvalidCredentials)

	_, err = m.Authenticate("nobody@example.com", "pa55word123")
	assert.Equal(t, err, ErrInvalidCredentials)
}

func TestUserModelGet(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	db := newTestDB(t)
	m := UserModel{DB: db}

	user, err := m.Get(1)
	assert.NilError(t, err)
	assert.Equal(t, user.Name, "Alice Jones")
	assert.Equal(t, user.Email, "alice@example.com")
	assert.Equal(t, user.Created.Format("2006-01-02 15:04:05"), "2022-01-01 09:18:24")
	assert.Equal(t, len(user.HashedPassword), 0)

	_, err = m.Get(99)
	assert.Equal(t, err, ErrNoRecord)
}

func TestUserModelPasswordUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	db := newTestDB(t)
	m := UserModel{DB: db}

	err := m.PasswordUpdate(1, "wrong", "newPa$$word")
	assert.Equal(t, err, ErrInvalidCredentials)

	err = m.PasswordUpdate(99, "pa$$word", "newPa$$word")
	assert.Equal(t, err, ErrNoRecord)

	// A failed attempt must not log anyone out.
	version, err := m.SessionVersion(1)
	assert.NilError(t, err)
	assert.Equal(t, version, 1)

	err = m.PasswordUpdate(1, "pa$$word", "newPa$$word")
	assert.NilError(t, err)

	_, err = m.Authenticate("alice@example.com", "pa$$word")
	assert.Equal(t, err, ErrInvalidCredentials)

	id, err := m.Authenticate("alice@example.com", "newPa$$word")
	assert.NilError(t, err)
	assert.Equal(t, id, 1)

	// A successful change bumps the session version, for Alice only.
	version, err = m.SessionVersion(1)
	assert.NilError(t, err)
	assert.Equal(t, version, 2)

	version, err = m.SessionVersion(2)
	assert.NilError(t, err)
	assert.Equal(t, version, 1)
}
