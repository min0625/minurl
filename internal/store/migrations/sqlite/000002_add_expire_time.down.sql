-- SQLite does not support DROP COLUMN on all supported versions.
-- Recreate the table without expire_time to reverse this migration.
CREATE TABLE short_urls_new (
    id TEXT PRIMARY KEY,
    original_url TEXT NOT NULL,
    create_time TEXT NOT NULL
);

INSERT INTO
    short_urls_new (id, original_url, create_time)
SELECT id, original_url, create_time
FROM short_urls;

DROP TABLE short_urls;

ALTER TABLE short_urls_new RENAME TO short_urls;
