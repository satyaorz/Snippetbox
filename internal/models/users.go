package models

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

// UserModelInterface describes the methods our handlers need from a user
// store, so that tests can swap in a mock implementation.
type UserModelInterface interface {
	Insert(name, email, password string) error
	Authenticate(email, password string) (int, error)
	SessionVersion(id int) (int, error)
	Get(id int) (User, error)
	PasswordUpdate(id int, currentPassword, newPassword string) error
}

// Define a User type. Notice how the field names and types align with the
// columns in the database "users" table?
type User struct {
	ID             int
	Name           string
	Email          string
	HashedPassword []byte
	Created        time.Time
}

// Define a UserModel type which wraps a database connection pool.
type UserModel struct {
	DB *sql.DB
}

// Insert adds a new record to the "users" table, storing a bcrypt hash of the
// plain-text password rather than the password itself.
func (m *UserModel) Insert(name, email, password string) error {
	// Create a bcrypt hash of the plain-text password. A cost of 12 is a
	// reasonable trade-off between security and speed.
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}

	stmt := `INSERT INTO users (name, email, hashed_password, created)
	VALUES(?, ?, ?, UTC_TIMESTAMP())`

	_, err = m.DB.Exec(stmt, name, email, string(hashedPassword))
	if err != nil {
		// If this returns an error, we use errors.As() to check whether it has
		// the type *mysql.MySQLError. If it does, and it relates to our
		// users_uc_email unique constraint (error number 1062), we return
		// ErrDuplicateEmail instead.
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1062 && strings.Contains(mySQLError.Message, "users_uc_email") {
				return ErrDuplicateEmail
			}
		}
		return err
	}

	return nil
}

// Authenticate verifies whether a user exists with the provided email address
// and password. It returns the relevant user ID if they do.
func (m *UserModel) Authenticate(email, password string) (int, error) {
	var id int
	var hashedPassword []byte

	stmt := "SELECT id, hashed_password FROM users WHERE email = ?"

	err := m.DB.QueryRow(stmt, email).Scan(&id, &hashedPassword)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrInvalidCredentials
		}
		return 0, err
	}

	// Check whether the hashed password and plain-text password provided match.
	err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return 0, ErrInvalidCredentials
		}
		return 0, err
	}

	return id, nil
}

// SessionVersion returns the user's current session version, or ErrNoRecord
// if the user doesn't exist. The version is stored in the session at login
// and bumped whenever the password changes, so comparing the two tells us
// whether a session is still valid.
func (m *UserModel) SessionVersion(id int) (int, error) {
	var version int

	stmt := "SELECT session_version FROM users WHERE id = ?"

	err := m.DB.QueryRow(stmt, id).Scan(&version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNoRecord
		}
		return 0, err
	}

	return version, nil
}

// Get returns the details for a specific user (without the password hash).
func (m *UserModel) Get(id int) (User, error) {
	var user User

	stmt := `SELECT id, name, email, created FROM users WHERE id = ?`

	err := m.DB.QueryRow(stmt, id).Scan(&user.ID, &user.Name, &user.Email, &user.Created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNoRecord
		}
		return User{}, err
	}

	return user, nil
}

// PasswordUpdate changes a user's password, but only if currentPassword
// matches the one we have stored. If it doesn't, ErrInvalidCredentials is
// returned and nothing is changed. A successful update also increments the
// user's session version.
func (m *UserModel) PasswordUpdate(id int, currentPassword, newPassword string) error {
	var currentHashedPassword []byte

	stmt := "SELECT hashed_password FROM users WHERE id = ?"

	err := m.DB.QueryRow(stmt, id).Scan(&currentHashedPassword)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoRecord
		}
		return err
	}

	err = bcrypt.CompareHashAndPassword(currentHashedPassword, []byte(currentPassword))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrInvalidCredentials
		}
		return err
	}

	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		return err
	}

	// Bump the session version too, which logs out every other session that
	// belongs to this user.
	stmt = "UPDATE users SET hashed_password = ?, session_version = session_version + 1 WHERE id = ?"

	_, err = m.DB.Exec(stmt, string(newHashedPassword), id)
	return err
}
