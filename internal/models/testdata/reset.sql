-- Drops everything the migrations create, so each test run starts from an
-- empty database. Also used to clean up afterwards.
DROP TABLE IF EXISTS tokens;
DROP TABLE IF EXISTS snippets;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS schema_migrations;
