PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

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

CREATE INDEX IF NOT EXISTS idx_message_date ON message(mailbox_id, date DESC);

-- CREATE VIRTUAL TABLE IF NOT EXISTS message_fts USING fts5(
--     subject,
--     body,
--     content='message',
--     content_rowid='id',
--     tokenize='porter unicode61'
-- );

-- CREATE TABLE IF NOT EXISTS attachments (
--     id INTEGER PRIMARY KEY,
--     message_id INTEGER REFERENCES messages(id) ON DELETE CASCADE,
--     filename TEXT,
--     content_type TEXT
-- );

-- CREATE TRIGGER IF NOT EXISTS message_ai AFTER INSERT ON message BEGIN
--     INSERT INTO message_fts(rowid, subject, body_text)
--     VALUES (new.id, new.subject, new.body_text);
-- END;

-- CREATE TRIGGER IF NOT EXISTS message_ad AFTER DELETE ON message BEGIN
--     DELETE FROM message_fts WHERE rowid = old.id;
-- END;

-- CREATE TRIGGER IF NOT EXISTS message_au AFTER UPDATE ON message BEGIN
--     UPDATE message_fts
--     SET subject = new.subject,
--         body = new.body
--     WHERE rowid = old.id;
-- END;
