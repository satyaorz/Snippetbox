package mocks

import (
	"strings"
	"time"

	"github.com/satyaorz/snippetbox/internal/models"
)

// Fixed snippets used by the handler tests:
//   - mockSnippet: public, owned by user 1 (Alice).
//   - privateSnippet: private, owned by user 2.
var (
	mockSnippet = models.Snippet{
		ID:         1,
		Slug:       "a1b2c3d4e5f6",
		UserID:     1,
		AuthorName: "Alice",
		Title:      "An old silent pond",
		Content:    "An old silent pond...",
		Language:   "",
		Visibility: models.VisibilityPublic,
		Created:    time.Now(),
		Expires:    time.Now(),
	}

	privateSnippet = models.Snippet{
		ID:         2,
		Slug:       "0f0f0f0f0f0f",
		UserID:     2,
		AuthorName: "Bob",
		Title:      "Bob's secret",
		Content:    "Shh",
		Visibility: models.VisibilityPrivate,
		Created:    time.Now(),
		Expires:    time.Now(),
	}
)

// SnippetModel is an in-memory stand-in for models.SnippetModel.
type SnippetModel struct{}

func (m *SnippetModel) Insert(userID int, title, content, language, visibility string, expires int) (string, error) {
	return "ffffffffffff", nil
}

func (m *SnippetModel) Get(slug string) (models.Snippet, error) {
	switch slug {
	case mockSnippet.Slug:
		return mockSnippet, nil
	case privateSnippet.Slug:
		return privateSnippet, nil
	default:
		return models.Snippet{}, models.ErrNoRecord
	}
}

// List returns mockSnippet (the only public snippet) when there's no search
// term or the term appears in its title.
func (m *SnippetModel) List(query string, page, pageSize int) ([]models.Snippet, int, error) {
	if page == 1 && (query == "" || strings.Contains(strings.ToLower(mockSnippet.Title), strings.ToLower(query))) {
		return []models.Snippet{mockSnippet}, 1, nil
	}
	if query == "" {
		return nil, 1, nil
	}
	return nil, 0, nil
}

func (m *SnippetModel) ListByUser(userID int) ([]models.Snippet, error) {
	switch userID {
	case 1:
		return []models.Snippet{mockSnippet}, nil
	case 2:
		return []models.Snippet{privateSnippet}, nil
	default:
		return nil, nil
	}
}

func (m *SnippetModel) Update(slug string, userID int, title, content, language, visibility string) error {
	return m.owned(slug, userID)
}

func (m *SnippetModel) Delete(slug string, userID int) error {
	return m.owned(slug, userID)
}

func (m *SnippetModel) owned(slug string, userID int) error {
	s, err := m.Get(slug)
	if err != nil || s.UserID != userID {
		return models.ErrNoRecord
	}
	return nil
}

func (m *SnippetModel) DeleteExpired() (int64, error) {
	return 0, nil
}
