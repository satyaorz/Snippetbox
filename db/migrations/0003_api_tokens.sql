-- Phase 4: bearer tokens for the JSON API. Only a SHA-256 hash of each token
-- is stored, so a database leak doesn't leak usable tokens.

CREATE TABLE tokens (
    hash BINARY(32) NOT NULL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    name VARCHAR(100) NOT NULL DEFAULT '',
    created DATETIME NOT NULL,
    expires DATETIME NOT NULL,
    INDEX idx_tokens_user (user_id),
    INDEX idx_tokens_expires (expires),
    CONSTRAINT tokens_fk_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
