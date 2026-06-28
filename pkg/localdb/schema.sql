PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;

CREATE TABLE IF NOT EXISTS mailbox (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    uid_validity INTEGER DEFAULT 0,
    uid_next INTEGER DEFAULT 1,
    highest_modseq INTEGER DEFAULT 0,
    last_sync DATETIME,
    UNIQUE(name)
);

CREATE TABLE IF NOT EXISTS message (
    id INTEGER PRIMARY KEY,
    mailbox_id INTEGER NOT NULL REFERENCES mailbox(id) ON DELETE CASCADE,
    uid INTEGER NOT NULL,
    message_id TEXT,
    path TEXT,
    flags TEXT DEFAULT '',
    modseq INTEGER DEFAULT 0,
    subject TEXT,
    from_name TEXT,
    from_address TEXT,
    date DATETIME,
    body TEXT,
    UNIQUE(mailbox_id, uid)
);

-- CREATE TABLE IF NOT EXISTS attachments (
--     id INTEGER PRIMARY KEY,
--     message_id INTEGER REFERENCES messages(id) ON DELETE CASCADE,
--     filename TEXT,
--     content_type TEXT
-- );

CREATE INDEX IF NOT EXISTS idx_message_date ON message(mailbox_id, date DESC);

CREATE VIRTUAL TABLE IF NOT EXISTS message_fts USING fts5(
    subject,
    body,
    content='message',
    content_rowid='id',
    tokenize='trigram'
);

CREATE TRIGGER IF NOT EXISTS message_ai AFTER INSERT ON message BEGIN
    INSERT INTO message_fts(rowid, subject, body)
    VALUES (new.id, new.subject, new.body);
END;

CREATE TRIGGER IF NOT EXISTS message_ad AFTER DELETE ON message BEGIN
    INSERT INTO message_fts(message_fts, rowid, subject, body)
    VALUES ('delete', old.id, old.subject, old.body);
END;

CREATE TRIGGER IF NOT EXISTS message_au AFTER UPDATE ON message BEGIN
    INSERT INTO message_fts(message_fts, rowid, subject, body)
    VALUES ('delete', old.id, old.subject, old.body);
    INSERT INTO message_fts(rowid, subject, body)
    VALUES (new.id, new.subject, new.body);
END;
