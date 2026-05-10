CREATE TABLE IF NOT EXISTS short_urls (
    id TEXT PRIMARY KEY,
    original_url TEXT NOT NULL,
    create_time TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS counters (
    name TEXT PRIMARY KEY,
    value INTEGER NOT NULL
);

INSERT OR IGNORE INTO counters (name, value) VALUES ('short_url', 0);
