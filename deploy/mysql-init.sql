-- Runs once when the MySQL container's data volume is first created.
CREATE USER IF NOT EXISTS 'web'@'%' IDENTIFIED BY 'pass';
GRANT SELECT, INSERT, UPDATE, DELETE ON snippetbox.* TO 'web'@'%';
