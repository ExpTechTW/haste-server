-- Share codes are allocated from a counter rather than derived from rowid,
-- because the code has to be known before the row is written: pastes are
-- immutable, so there is no second pass to fill it in.
CREATE TABLE IF NOT EXISTS counters (
    name  TEXT    PRIMARY KEY,
    value INTEGER NOT NULL
) STRICT;

INSERT OR IGNORE INTO counters (name, value) VALUES ('paste_seq', 0);

CREATE TABLE IF NOT EXISTS pastes (
    seq        INTEGER PRIMARY KEY,      -- allocated from counters.paste_seq
    code       TEXT    NOT NULL,         -- share code, permutation of seq
    body       BLOB    NOT NULL,         -- zstd frame
    codec      INTEGER NOT NULL,         -- which codec produced body
    chars      INTEGER NOT NULL,         -- runes, as counted against the limit
    raw_bytes  INTEGER NOT NULL,         -- decoded UTF-8 length
    language   TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,         -- unix seconds
    expires_at INTEGER NOT NULL          -- unix seconds; 0 = never expires
) STRICT;

CREATE UNIQUE INDEX IF NOT EXISTS pastes_code_idx ON pastes (code);

-- Partial index: rows that never expire are the ones the sweeper must not scan.
CREATE INDEX IF NOT EXISTS pastes_expires_idx ON pastes (expires_at) WHERE expires_at > 0;

-- A paste is write-once. Enforced in the database, not just in the handlers, so
-- no future code path (or sqlite3 shell) can quietly rewrite a published code.
CREATE TRIGGER IF NOT EXISTS pastes_immutable
BEFORE UPDATE ON pastes
BEGIN
    SELECT RAISE(ABORT, 'pastes are immutable');
END;
