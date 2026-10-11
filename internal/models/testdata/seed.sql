-- Resets the test tables to a known state. Run before every test by
-- newTestDB(). Deleting rows and resetting AUTO_INCREMENT is much faster than
-- dropping and recreating the tables (the FULLTEXT index makes that slow).
DELETE FROM snippets;
DELETE FROM users;
ALTER TABLE users AUTO_INCREMENT = 1;
ALTER TABLE snippets AUTO_INCREMENT = 1;

-- hashed_password is the bcrypt hash (cost 12) of "pa$$word".
INSERT INTO users (name, email, hashed_password, created) VALUES
    ('Alice Jones', 'alice@example.com', '$2a$12$LXFGpmGCepzIAE5ftkRCLO5rGrrMKCuTjubErAaJB2RmAENu4gn.e', '2022-01-01 09:18:24'),
    ('Bob Smith', 'bob@example.com', '$2a$12$LXFGpmGCepzIAE5ftkRCLO5rGrrMKCuTjubErAaJB2RmAENu4gn.e', '2022-01-02 10:00:00');

INSERT INTO snippets (slug, user_id, title, content, language, visibility, created, expires) VALUES
    ('aaaaaaaaaaa1', 1, 'An old silent pond', 'An old silent pond...', '', 'public', UTC_TIMESTAMP(), DATE_ADD(UTC_TIMESTAMP(), INTERVAL 365 DAY)),
    ('aaaaaaaaaaa2', 1, 'Unlisted frog', 'A frog jumps into the pond', 'go', 'unlisted', UTC_TIMESTAMP(), DATE_ADD(UTC_TIMESTAMP(), INTERVAL 365 DAY)),
    ('aaaaaaaaaaa3', 2, 'Private splash', 'Splash! Silence again.', '', 'private', UTC_TIMESTAMP(), DATE_ADD(UTC_TIMESTAMP(), INTERVAL 365 DAY)),
    ('aaaaaaaaaaa4', NULL, 'Expired pond', 'Long gone pond', '', 'public', DATE_SUB(UTC_TIMESTAMP(), INTERVAL 2 DAY), DATE_SUB(UTC_TIMESTAMP(), INTERVAL 1 DAY));
