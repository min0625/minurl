CREATE TABLE IF NOT EXISTS short_urls (
    -- COLLATE utf8mb4_0900_as_cs makes the primary key lookup case-sensitive
    -- so that "abcdef" and "ABCDEF" are treated as distinct IDs.
    id VARCHAR(255) NOT NULL COLLATE utf8mb4_0900_as_cs,
    original_url TEXT NOT NULL,
    create_time DATETIME(6) NOT NULL,
    PRIMARY KEY (id)
) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS counters (
    name VARCHAR(255) NOT NULL COLLATE utf8mb4_0900_as_cs,
    value BIGINT NOT NULL,
    PRIMARY KEY (name)
) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

INSERT IGNORE INTO counters (name, value) VALUES ('short_url', 0);
