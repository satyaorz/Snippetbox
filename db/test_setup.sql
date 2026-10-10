-- Database and user for the model integration tests (go test ./internal/models).
-- Run as a privileged user:  mysql -uroot < db/test_setup.sql

CREATE DATABASE IF NOT EXISTS test_snippetbox CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

CREATE USER IF NOT EXISTS 'test_web'@'localhost' IDENTIFIED BY 'pass';
GRANT CREATE, DROP, ALTER, INDEX, SELECT, INSERT, UPDATE, DELETE ON test_snippetbox.* TO 'test_web'@'localhost';
