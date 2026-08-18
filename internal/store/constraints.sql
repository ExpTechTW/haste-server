-- Applied after schema.sql and after Open() has added any column an older
-- database is missing. Everything here names expires_at, which a database
-- created before temporary pastes existed does not have yet, so it cannot live
-- in schema.sql: SQLite validates a trigger's column list when it is created.

-- Sweeping expired pastes reads only rows that have an expiry, which on an
-- instance where most pastes are permanent is a small fraction of the table.
CREATE INDEX IF NOT EXISTS pastes_expires_idx
    ON pastes (expires_at) WHERE expires_at IS NOT NULL;

-- A paste's content is write-once, enforced in the database rather than only in
-- the handlers, so no future code path (or sqlite3 shell) can quietly rewrite a
-- code that has already been shared.
--
-- accessed_at is deliberately absent from the column list: it is bookkeeping,
-- not content, and LRU eviction needs it to move. Everything that a reader
-- could actually observe stays frozen — including expires_at, because a
-- lifetime chosen at save time is a promise to whoever holds the link.
--
-- Dropped and recreated rather than IF NOT EXISTS, so the guarded column list
-- always matches this file. A trigger created by an older build would otherwise
-- survive forever, silently protecting fewer columns than it appears to.
DROP TRIGGER IF EXISTS pastes_immutable;

CREATE TRIGGER pastes_immutable
BEFORE UPDATE OF seq, code, body, codec, bytes, chars, raw_bytes, language, created_at, expires_at
ON pastes
BEGIN
    SELECT RAISE(ABORT, 'pastes are immutable');
END;
