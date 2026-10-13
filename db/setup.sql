-- Creates the database and the application's MySQL user. Run once as a
-- privileged user:  mysql -uroot < db/setup.sql
-- Then create the tables with:  go run ./cmd/migrate -dsn 'root@/snippetbox' up

CREATE DATABASE IF NOT EXISTS snippetbox CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- The web app only needs to read and write rows; schema changes are made by
-- the migrate command, which runs with a more privileged account.
-- (Use a real password for anything beyond local development.)
CREATE USER IF NOT EXISTS 'web'@'localhost' IDENTIFIED BY 'pass';
GRANT SELECT, INSERT, UPDATE, DELETE ON snippetbox.* TO 'web'@'localhost';
