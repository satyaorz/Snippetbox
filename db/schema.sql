-- Snippetbox database schema (MySQL 8+) for a fresh install.
-- Run as a privileged user:  mysql -uroot < db/schema.sql
-- Existing databases should apply the files in db/migrations/ instead.

CREATE DATABASE IF NOT EXISTS snippetbox CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE snippetbox;

CREATE TABLE IF NOT EXISTS users (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    hashed_password CHAR(60) NOT NULL,
    created DATETIME NOT NULL,
    -- Bumped whenever the password changes; sessions created with an older
    -- version are treated as logged out.
    session_version INTEGER NOT NULL DEFAULT 1,
    CONSTRAINT users_uc_email UNIQUE (email)
);

CREATE TABLE IF NOT EXISTS snippets (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    -- Random public identifier used in URLs, so snippets can't be enumerated.
    slug CHAR(12) NOT NULL,
    -- NULL for anonymous snippets created before accounts existed.
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

-- Session store used by github.com/alexedwards/scs/mysqlstore.
CREATE TABLE IF NOT EXISTS sessions (
    token CHAR(43) PRIMARY KEY,
    data BLOB NOT NULL,
    expiry TIMESTAMP(6) NOT NULL,
    INDEX sessions_expiry_idx (expiry)
);

-- Application user (adjust the password for anything beyond local dev).
CREATE USER IF NOT EXISTS 'web'@'localhost' IDENTIFIED BY 'pass';
GRANT SELECT, INSERT, UPDATE, DELETE ON snippetbox.* TO 'web'@'localhost';
