package models

import (
	"testing"

	"github.com/satyaorz/snippetbox/internal/assert"

	_ "github.com/go-sql-driver/mysql"
)

func TestUserModelExists(t *testing.T) {
	// Integration tests need a real MySQL database, so skip them when running
	// with the -short flag.
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	tests := []struct {
		name   string
		userID int
		want   bool
	}{
		{name: "Valid ID", userID: 1, want: true},
		{name: "Zero ID", userID: 0, want: false},
		{name: "Non-existent ID", userID: 2, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			m := UserModel{DB: db}

			exists, err := m.Exists(tt.userID)

			assert.Equal(t, exists, tt.want)
			assert.NilError(t, err)
		})
	}
}

func TestUserModelInsertAndAuthenticate(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	db := newTestDB(t)
	m := UserModel{DB: db}

	err := m.Insert("Bob", "bob@example.com", "pa55word123")
	assert.NilError(t, err)

	err = m.Insert("Alice again", "alice@example.com", "whatever123")
	assert.Equal(t, err, ErrDuplicateEmail)

	id, err := m.Authenticate("bob@example.com", "pa55word123")
	assert.NilError(t, err)
	assert.Equal(t, id, 2)

	_, err = m.Authenticate("bob@example.com", "wrong")
	assert.Equal(t, err, ErrInvalidCredentials)

	_, err = m.Authenticate("nobody@example.com", "pa55word123")
	assert.Equal(t, err, ErrInvalidCredentials)
}

func TestSnippetModel(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	db := newTestDB(t)
	m := SnippetModel{DB: db}

	s, err := m.Get(1)
	assert.NilError(t, err)
	assert.Equal(t, s.Title, "An old silent pond")

	_, err = m.Get(99)
	assert.Equal(t, err, ErrNoRecord)

	id, err := m.Insert("Second", "content", 7)
	assert.NilError(t, err)
	assert.Equal(t, id, 2)

	latest, err := m.Latest()
	assert.NilError(t, err)
	assert.Equal(t, len(latest), 2)
	assert.Equal(t, latest[0].ID, 2)
}
