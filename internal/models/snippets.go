package models

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Snippet visibility levels.
//   - public: listed on the home page and in search results.
//   - unlisted: viewable by anyone with the link, but never listed.
//   - private: only viewable by the snippet's owner.
const (
	VisibilityPublic   = "public"
	VisibilityUnlisted = "unlisted"
	VisibilityPrivate  = "private"
)

// SnippetModelInterface describes the methods our handlers need from a
// snippet store, so that tests can swap in a mock implementation.
type SnippetModelInterface interface {
	Insert(userID int, title, content, language, visibility string, expires int) (string, error)
	Get(slug string) (Snippet, error)
	List(query string, page, pageSize int) ([]Snippet, int, error)
	ListByUser(userID int) ([]Snippet, error)
	Update(slug string, userID int, title, content, language, visibility string) error
	Delete(slug string, userID int) error
	DeleteExpired() (int64, error)
}

// Define a Snippet type to hold the data for an individual snippet. Notice how
// the fields of the struct correspond to the fields in our MySQL snippets
// table?
type Snippet struct {
	ID         int
	Slug       string
	UserID     int // 0 for anonymous snippets.
	AuthorName string
	Title      string
	Content    string
	Language   string
	Visibility string
	Created    time.Time
	Expires    time.Time
}

// Define a SnippetModel type which wraps a sql.DB connection pool.
type SnippetModel struct {
	DB *sql.DB
}

// The columns selected for every snippet query. The LEFT JOIN brings in the
// author's name; COALESCE turns the NULLs for anonymous snippets into zero
// values so they can be scanned straight into an int and a string.
const snippetColumns = `s.id, s.slug, COALESCE(s.user_id, 0), COALESCE(u.name, ''),
	s.title, s.content, s.language, s.visibility, s.created, s.expires`

const snippetFrom = `FROM snippets s LEFT JOIN users u ON u.id = s.user_id`

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanSnippet(row scanner) (Snippet, error) {
	var s Snippet
	err := row.Scan(&s.ID, &s.Slug, &s.UserID, &s.AuthorName,
		&s.Title, &s.Content, &s.Language, &s.Visibility, &s.Created, &s.Expires)
	return s, err
}

// newSlug returns a random 12 character hex string (48 bits of entropy) to use
// as a snippet's public identifier.
func newSlug() (string, error) {
	b := make([]byte, 6)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Insert creates a new snippet owned by userID and returns its slug.
func (m *SnippetModel) Insert(userID int, title, content, language, visibility string, expires int) (string, error) {
	stmt := `INSERT INTO snippets (slug, user_id, title, content, language, visibility, created, expires)
	VALUES(?, ?, ?, ?, ?, ?, UTC_TIMESTAMP(), DATE_ADD(UTC_TIMESTAMP(), INTERVAL ? DAY))`

	// Slugs are random, so a collision is extremely unlikely, but if one does
	// happen the unique constraint rejects the insert and we try again with a
	// fresh slug.
	for range 3 {
		slug, err := newSlug()
		if err != nil {
			return "", err
		}

		_, err = m.DB.Exec(stmt, slug, userID, title, content, language, visibility, expires)
		if err == nil {
			return slug, nil
		}

		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) && mySQLError.Number == 1062 && strings.Contains(mySQLError.Message, "snippets_uc_slug") {
			continue
		}
		return "", err
	}

	return "", errors.New("models: could not generate a unique snippet slug")
}

// Get returns a non-expired snippet by its slug, whatever its visibility.
// Callers are responsible for checking whether the current user may see it.
func (m *SnippetModel) Get(slug string) (Snippet, error) {
	stmt := `SELECT ` + snippetColumns + ` ` + snippetFrom + `
	WHERE s.expires > UTC_TIMESTAMP() AND s.slug = ?`

	s, err := scanSnippet(m.DB.QueryRow(stmt, slug))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snippet{}, ErrNoRecord
		}
		return Snippet{}, err
	}

	return s, nil
}

// List returns one page of public, non-expired snippets, newest first, along
// with the total number of matching snippets. If query is not empty, only
// snippets whose title or content match it (using MySQL full-text search) are
// returned, ordered by relevance.
func (m *SnippetModel) List(query string, page, pageSize int) ([]Snippet, int, error) {
	where := `WHERE s.expires > UTC_TIMESTAMP() AND s.visibility = 'public'`
	orderBy := `ORDER BY s.id DESC`
	args := []any{}

	query = strings.TrimSpace(query)
	if query != "" {
		where += ` AND MATCH(s.title, s.content) AGAINST(? IN NATURAL LANGUAGE MODE)`
		orderBy = `ORDER BY MATCH(s.title, s.content) AGAINST(? IN NATURAL LANGUAGE MODE) DESC, s.id DESC`
		args = append(args, query)
	}

	var total int
	err := m.DB.QueryRow(`SELECT COUNT(*) `+snippetFrom+` `+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	stmt := `SELECT ` + snippetColumns + ` ` + snippetFrom + ` ` + where + ` ` + orderBy + ` LIMIT ? OFFSET ?`

	listArgs := append([]any{}, args...)
	if query != "" {
		// The ORDER BY clause needs the search term a second time.
		listArgs = append(listArgs, query)
	}
	listArgs = append(listArgs, pageSize, (page-1)*pageSize)

	snippets, err := m.query(stmt, listArgs...)
	if err != nil {
		return nil, 0, err
	}

	return snippets, total, nil
}

// ListByUser returns all of a user's non-expired snippets (of any visibility),
// newest first.
func (m *SnippetModel) ListByUser(userID int) ([]Snippet, error) {
	stmt := `SELECT ` + snippetColumns + ` ` + snippetFrom + `
	WHERE s.expires > UTC_TIMESTAMP() AND s.user_id = ? ORDER BY s.id DESC`

	return m.query(stmt, userID)
}

func (m *SnippetModel) query(stmt string, args ...any) ([]Snippet, error) {
	rows, err := m.DB.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	// We defer rows.Close() to ensure the sql.Rows resultset is always
	// properly closed before the method returns.
	defer rows.Close()

	var snippets []Snippet

	for rows.Next() {
		s, err := scanSnippet(rows)
		if err != nil {
			return nil, err
		}
		snippets = append(snippets, s)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}
	return snippets, nil
}

// Update changes a snippet's editable fields. The WHERE clause also matches
// on user_id, so a user can only ever update their own snippets; anything
// else returns ErrNoRecord.
func (m *SnippetModel) Update(slug string, userID int, title, content, language, visibility string) error {
	stmt := `UPDATE snippets SET title = ?, content = ?, language = ?, visibility = ?
	WHERE slug = ? AND user_id = ? AND expires > UTC_TIMESTAMP()`

	result, err := m.DB.Exec(stmt, title, content, language, visibility, slug, userID)
	if err != nil {
		return err
	}

	return m.checkAffected(slug, userID, result)
}

// Delete removes one of the user's own snippets. Like Update, it returns
// ErrNoRecord if the snippet doesn't exist or belongs to someone else.
func (m *SnippetModel) Delete(slug string, userID int) error {
	stmt := `DELETE FROM snippets WHERE slug = ? AND user_id = ?`

	result, err := m.DB.Exec(stmt, slug, userID)
	if err != nil {
		return err
	}

	return m.checkAffected(slug, userID, result)
}

// checkAffected turns "no rows changed" into ErrNoRecord. MySQL reports 0
// affected rows for an UPDATE that matched a row but didn't change any values,
// so in that case we check whether the row exists before deciding.
func (m *SnippetModel) checkAffected(slug string, userID int, result sql.Result) error {
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	var exists bool
	stmt := `SELECT EXISTS(SELECT true FROM snippets WHERE slug = ? AND user_id = ? AND expires > UTC_TIMESTAMP())`
	err = m.DB.QueryRow(stmt, slug, userID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNoRecord
	}
	return nil
}

// DeleteExpired permanently removes every expired snippet and returns how
// many were deleted.
func (m *SnippetModel) DeleteExpired() (int64, error) {
	result, err := m.DB.Exec(`DELETE FROM snippets WHERE expires <= UTC_TIMESTAMP()`)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}
