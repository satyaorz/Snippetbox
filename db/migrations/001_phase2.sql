-- Upgrades a database created from the original schema to Phase 2:
-- snippet ownership, random slugs, visibility, languages, full-text search
-- and per-user session versions.
-- Run once as a privileged user:  mysql -uroot snippetbox < db/migrations/001_phase2.sql

ALTER TABLE users
    ADD COLUMN session_version INTEGER NOT NULL DEFAULT 1;

ALTER TABLE snippets
    ADD COLUMN slug CHAR(12) NULL AFTER id,
    ADD COLUMN user_id INTEGER NULL AFTER slug,
    ADD COLUMN language VARCHAR(32) NOT NULL DEFAULT '' AFTER content,
    ADD COLUMN visibility ENUM('public', 'unlisted', 'private') NOT NULL DEFAULT 'public' AFTER language;

-- Give existing snippets a random slug. RANDOM_BYTES() is cryptographically
-- secure; the unique constraint below catches the (astronomically unlikely)
-- collision.
UPDATE snippets SET slug = LOWER(HEX(RANDOM_BYTES(6))) WHERE slug IS NULL;

ALTER TABLE snippets
    MODIFY slug CHAR(12) NOT NULL,
    ADD CONSTRAINT snippets_uc_slug UNIQUE (slug),
    ADD CONSTRAINT snippets_fk_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    ADD INDEX idx_snippets_expires (expires);

ALTER TABLE snippets
    ADD FULLTEXT INDEX snippets_ft_title_content (title, content);
