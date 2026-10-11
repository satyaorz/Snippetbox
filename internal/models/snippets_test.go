package models

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/satyaorz/snippetbox/internal/assert"
)

// The seed data (testdata/setup.sql) contains four snippets:
//
//	aaaaaaaaaaa1  Alice  public    "An old silent pond"
//	aaaaaaaaaaa2  Alice  unlisted  "Unlisted frog" (language go)
//	aaaaaaaaaaa3  Bob    private   "Private splash"
//	aaaaaaaaaaa4  -      public    "Expired pond" (already expired)

func TestSnippetModelGet(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	db := newTestDB(t)
	m := SnippetModel{DB: db}

	s, err := m.Get("aaaaaaaaaaa1")
	assert.NilError(t, err)
	assert.Equal(t, s.Title, "An old silent pond")
	assert.Equal(t, s.UserID, 1)
	assert.Equal(t, s.AuthorName, "Alice Jones")
	assert.Equal(t, s.Visibility, VisibilityPublic)

	// Get returns private snippets too; access control is the handler's job.
	s, err = m.Get("aaaaaaaaaaa3")
	assert.NilError(t, err)
	assert.Equal(t, s.Visibility, VisibilityPrivate)

	_, err = m.Get("aaaaaaaaaaa4")
	assert.Equal(t, err, ErrNoRecord)

	_, err = m.Get("doesnotexist")
	assert.Equal(t, err, ErrNoRecord)
}

func TestSnippetModelInsert(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	db := newTestDB(t)
	m := SnippetModel{DB: db}

	slug, err := m.Insert(2, "Bob's haiku", "Lines of code", "go", VisibilityUnlisted, 7)
	assert.NilError(t, err)
	assert.Equal(t, regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(slug), true)

	s, err := m.Get(slug)
	assert.NilError(t, err)
	assert.Equal(t, s.UserID, 2)
	assert.Equal(t, s.AuthorName, "Bob Smith")
	assert.Equal(t, s.Language, "go")
	assert.Equal(t, s.Visibility, VisibilityUnlisted)

	other, err := m.Insert(2, "Another", "content", "", VisibilityPublic, 1)
	assert.NilError(t, err)
	assert.Equal(t, other != slug, true)
}

func TestSnippetModelList(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	t.Run("Only public, non-expired snippets", func(t *testing.T) {
		db := newTestDB(t)
		m := SnippetModel{DB: db}

		snippets, total, err := m.List("", 1, 10)
		assert.NilError(t, err)
		assert.Equal(t, total, 1)
		assert.Equal(t, len(snippets), 1)
		assert.Equal(t, snippets[0].Slug, "aaaaaaaaaaa1")
	})

	t.Run("Search", func(t *testing.T) {
		db := newTestDB(t)
		m := SnippetModel{DB: db}

		_, err := m.Insert(1, "Basho", "The frog leaps into the silent water", "", VisibilityPublic, 7)
		assert.NilError(t, err)

		snippets, total, err := m.List("silent", 1, 10)
		assert.NilError(t, err)
		assert.Equal(t, total, 2)
		assert.Equal(t, len(snippets), 2)

		// "frog" appears in the unlisted snippet too, which must not leak.
		snippets, total, err = m.List("frog", 1, 10)
		assert.NilError(t, err)
		assert.Equal(t, total, 1)
		assert.Equal(t, snippets[0].Title, "Basho")

		snippets, total, err = m.List("nonexistentword", 1, 10)
		assert.NilError(t, err)
		assert.Equal(t, total, 0)
		assert.Equal(t, len(snippets), 0)
	})

	t.Run("Pagination", func(t *testing.T) {
		db := newTestDB(t)
		m := SnippetModel{DB: db}

		for i := range 4 {
			_, err := m.Insert(1, fmt.Sprintf("Extra %d", i), "content", "", VisibilityPublic, 7)
			assert.NilError(t, err)
		}

		// 5 public snippets in pages of 2: newest first.
		page1, total, err := m.List("", 1, 2)
		assert.NilError(t, err)
		assert.Equal(t, total, 5)
		assert.Equal(t, len(page1), 2)
		assert.Equal(t, page1[0].Title, "Extra 3")

		page3, _, err := m.List("", 3, 2)
		assert.NilError(t, err)
		assert.Equal(t, len(page3), 1)
		assert.Equal(t, page3[0].Slug, "aaaaaaaaaaa1")
	})
}

func TestSnippetModelListByUser(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	db := newTestDB(t)
	m := SnippetModel{DB: db}

	snippets, err := m.ListByUser(1)
	assert.NilError(t, err)
	assert.Equal(t, len(snippets), 2) // public + unlisted
	assert.Equal(t, snippets[0].Slug, "aaaaaaaaaaa2")

	snippets, err = m.ListByUser(2)
	assert.NilError(t, err)
	assert.Equal(t, len(snippets), 1) // private snippets are listed for their owner

	snippets, err = m.ListByUser(99)
	assert.NilError(t, err)
	assert.Equal(t, len(snippets), 0)
}

func TestSnippetModelUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	db := newTestDB(t)
	m := SnippetModel{DB: db}

	// Bob can't edit Alice's snippet.
	err := m.Update("aaaaaaaaaaa1", 2, "Hacked", "Hacked", "", VisibilityPublic)
	assert.Equal(t, err, ErrNoRecord)

	err = m.Update("aaaaaaaaaaa1", 1, "Edited", "New content", "python", VisibilityPrivate)
	assert.NilError(t, err)

	s, err := m.Get("aaaaaaaaaaa1")
	assert.NilError(t, err)
	assert.Equal(t, s.Title, "Edited")
	assert.Equal(t, s.Content, "New content")
	assert.Equal(t, s.Language, "python")
	assert.Equal(t, s.Visibility, VisibilityPrivate)

	// Saving without changing anything is not an error.
	err = m.Update("aaaaaaaaaaa1", 1, "Edited", "New content", "python", VisibilityPrivate)
	assert.NilError(t, err)

	err = m.Update("doesnotexist", 1, "x", "x", "", VisibilityPublic)
	assert.Equal(t, err, ErrNoRecord)
}

func TestSnippetModelDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	db := newTestDB(t)
	m := SnippetModel{DB: db}

	err := m.Delete("aaaaaaaaaaa1", 2)
	assert.Equal(t, err, ErrNoRecord)

	err = m.Delete("aaaaaaaaaaa1", 1)
	assert.NilError(t, err)

	_, err = m.Get("aaaaaaaaaaa1")
	assert.Equal(t, err, ErrNoRecord)

	err = m.Delete("aaaaaaaaaaa1", 1)
	assert.Equal(t, err, ErrNoRecord)
}

func TestSnippetModelDeleteExpired(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration test")
	}

	db := newTestDB(t)
	m := SnippetModel{DB: db}

	n, err := m.DeleteExpired()
	assert.NilError(t, err)
	assert.Equal(t, n, int64(1))

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM snippets").Scan(&count)
	assert.NilError(t, err)
	assert.Equal(t, count, 3)

	n, err = m.DeleteExpired()
	assert.NilError(t, err)
	assert.Equal(t, n, int64(0))
}
