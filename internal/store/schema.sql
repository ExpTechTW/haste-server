-- Share codes are allocated from a counter rather than derived from rowid,
-- because the code has to be known before the row is written: pastes are
-- immutable, so there is no second pass to fill it in.
--
-- stored_bytes is the live total of pastes.bytes. Keeping it as a counter makes
-- the space cap an O(1) check on the write path, which is what lets it be a
-- hard guarantee instead of an hourly approximation. Open() recomputes it from
-- the table on startup, so it self-heals if anything ever edits the file
-- outside this process.
CREATE TABLE IF NOT EXISTS counters (
    name  TEXT    PRIMARY KEY,
    value INTEGER NOT NULL
) STRICT;

INSERT OR IGNORE INTO counters (name, value) VALUES ('paste_seq', 0);
INSERT OR IGNORE INTO counters (name, value) VALUES ('stored_bytes', 0);

CREATE TABLE IF NOT EXISTS pastes (
    seq         INTEGER PRIMARY KEY,      -- allocated from counters.paste_seq
    code        TEXT    NOT NULL,         -- share code, permutation of seq
    body        BLOB    NOT NULL,         -- zstd frame
    codec       INTEGER NOT NULL,         -- which codec produced body
    bytes       INTEGER NOT NULL,         -- LENGTH(body), so accounting never
                                          -- has to touch blob overflow pages
    chars       INTEGER NOT NULL,         -- runes, as counted against the limit
    raw_bytes   INTEGER NOT NULL,         -- decoded UTF-8 length
    language    TEXT    NOT NULL DEFAULT '',
    title       TEXT    NOT NULL DEFAULT '',   -- optional, <= MaxTitleChars runes
    created_at  INTEGER NOT NULL,         -- unix seconds
    accessed_at INTEGER NOT NULL,         -- unix seconds; drives LRU eviction
    expires_at  INTEGER                   -- unix seconds; NULL = no expiry set
) STRICT;

CREATE UNIQUE INDEX IF NOT EXISTS pastes_code_idx ON pastes (code);

-- Eviction reads in least-recently-used order, and the access TTL sweeps by the
-- same column, so this index carries both.
CREATE INDEX IF NOT EXISTS pastes_accessed_idx ON pastes (accessed_at);
CREATE INDEX IF NOT EXISTS pastes_created_idx ON pastes (created_at);
