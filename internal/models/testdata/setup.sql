-- Creates the test tables. Run once per `go test` run by TestMain; the
-- DROPs clean up after a previous run that was interrupted.
DROP TABLE IF EXISTS snippets;
DROP TABLE IF EXISTS users;

CREATE TABLE users (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    hashed_password CHAR(60) NOT NULL,
    created DATETIME NOT NULL,
    session_version INTEGER NOT NULL DEFAULT 1,
    CONSTRAINT users_uc_email UNIQUE (email)
);

CREATE TABLE snippets (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    slug CHAR(12) NOT NULL,
    user_id INTEGER NULL,
    title VARCHAR(100) NOT NULL,
    content TEXT NOT NULL,
    language VARCHAR(32) NOT NULL DEFAULT '',
    visibility ENUM('public', 'unlisted', 'private') NOT NULL DEFAULT 'public',
    created DATETIME NOT NULL,
    expires DATETIME NOT NULL,
    INDEX idx_snippets_created (created),
    INDEX idx_snippets_expires (expires),
    CONSTRAINT snippets_uc_slug UNIQUE (slug),
    CONSTRAINT snippets_fk_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FULLTEXT INDEX snippets_ft_title_content (title, content)
);
