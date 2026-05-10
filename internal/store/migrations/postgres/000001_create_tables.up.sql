CREATE TABLE IF NOT EXISTS short_urls (
    id TEXT PRIMARY KEY,
    original_url TEXT NOT NULL,
    create_time TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS counters (
    name TEXT PRIMARY KEY,
    value BIGINT NOT NULL
);

INSERT INTO
    counters (name, value)
VALUES ('short_url', 0) ON CONFLICT DO NOTHING;
