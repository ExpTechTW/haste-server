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
    created_at  INTEGER NOT NULL,         -- unix seconds
    accessed_at INTEGER NOT NULL          -- unix seconds; drives LRU eviction
) STRICT;

CREATE UNIQUE INDEX IF NOT EXISTS pastes_code_idx ON pastes (code);

-- Eviction reads in least-recently-used order, and the access TTL sweeps by the
-- same column, so this index carries both.
CREATE INDEX IF NOT EXISTS pastes_accessed_idx ON pastes (accessed_at);
CREATE INDEX IF NOT EXISTS pastes_created_idx ON pastes (created_at);

-- A paste's content is write-once, enforced in the database rather than only in
-- the handlers, so no future code path (or sqlite3 shell) can quietly rewrite a
-- code that has already been shared.
--
-- accessed_at is deliberately absent from the column list: it is bookkeeping,
-- not content, and LRU eviction needs it to move. Everything that a reader
-- could actually observe stays frozen.
CREATE TRIGGER IF NOT EXISTS pastes_immutable
BEFORE UPDATE OF seq, code, body, codec, bytes, chars, raw_bytes, language, created_at
ON pastes
BEGIN
    SELECT RAISE(ABORT, 'pastes are immutable');
END;
