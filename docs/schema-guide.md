# SQLite Schema Guide — Puffin

## Core Concepts

- **`INTEGER PRIMARY KEY`** — SQLite auto-increments this. It's also the internal `ROWID` used by FTS5 for joins.
- **`UNIQUE`** — Prevents duplicate values. Creates an implicit index.
- **`REFERENCES ... ON DELETE CASCADE`** — Foreign key. If parent row is deleted, children are deleted too.
- **`BOOLEAN`** — SQLite has no real boolean type. It's stored as INTEGER (0 or 1).
- **`DATETIME`** — Stored as text `YYYY-MM-DD HH:MM:SS` or as Unix epoch integers.
- **`BLOB`** — Raw binary data (encrypted passwords, etc).
- **`TEXT`** — Variable-length string. No length limit in practice.
- **`IF NOT EXISTS`** — Safe to run repeatedly; skips if object already exists.

---

## `accounts`

Stores email account credentials and connection info.

| Field | Type | Notes |
|-------|------|-------|
| `id` | `INTEGER PRIMARY KEY` | Auto-increment rowid. Referenced by mailboxes, threads, messages. |
| `email` | `TEXT UNIQUE NOT NULL` | Unique constraint prevents duplicate accounts. |
| `imap_host` | `TEXT NOT NULL` | e.g. `imap.gmail.com`. |
| `imap_port` | `INTEGER` | Usually 993 for IMAPS. |
| `username` | `TEXT NOT NULL` | Login name (often same as email). |
| `password_encrypted` | `BLOB` | Raw binary — stores encrypted password or OAuth token. |
| `sync_enabled` | `BOOLEAN DEFAULT 1` | INTEGER under the hood. 1 = sync, 0 = paused. |
| `last_sync` | `DATETIME` | Timestamp of last successful IMAP sync. |

---

## `mailboxes`

IMAP folders (INBOX, Sent, Archive) per account.

| Field | Type | Notes |
|-------|------|-------|
| `id` | `INTEGER PRIMARY KEY` | Auto-increment rowid. |
| `account_id` | `INTEGER REFERENCES accounts(id) ON DELETE CASCADE` | Foreign key. Deleting the account cascades to its mailboxes. |
| `name` | `TEXT NOT NULL` | Display name like "INBOX" or "Sent". |
| `path` | `TEXT` | IMAP folder path (sometimes differs from name). |
| `uid_validity` | `INTEGER` | IMAP UIDVALIDITY — changes when server resets UIDs. Critical for detecting stale cache. |
| `uid_next` | `INTEGER` | IMAP UIDNEXT — the next UID the server will assign. |
| `highest_modseq` | `INTEGER` | IMAP MODSEQ for CONDSTORE/QRESYNC — detects changes since last sync. |
| `last_sync` | `DATETIME` | Timestamp of last sync for this folder. |
| `UNIQUE(account_id, name)` | Composite constraint | Prevents duplicate mailbox names within one account. |

---

## `threads`

Groups messages belonging to the same conversation.

| Field | Type | Notes |
|-------|------|-------|
| `id` | `INTEGER PRIMARY KEY` | Auto-increment rowid. Referenced by `messages.thread_id`. |
| `account_id` | `INTEGER NOT NULL REFERENCES accounts(id)` | Which account this thread belongs to. |
| `thread_key` | `TEXT UNIQUE NOT NULL` | Unique identifier for the thread. Either Gmail's X-GM-THRID or the root Message-ID. |
| `subject_normalized` | `TEXT` | Subject with "Re:", "Fwd:" stripped for grouping. |
| `first_message_id` | `INTEGER` | Points to `messages.id` of the first email in the thread. |
| `last_message_date` | `DATETIME` | Date of the most recent message. Used for sorting thread list. |
| `message_count` | `INTEGER DEFAULT 0` | Total messages in this thread. |
| `unread_count` | `INTEGER DEFAULT 0` | How many messages are still unread. |
| `participants` | `TEXT` | JSON array of `{name, address}` objects for thread preview. |
| `snippet` | `TEXT` | Short preview text from the last message. |
| `updated_at` | `DATETIME DEFAULT CURRENT_TIMESTAMP` | Auto-set on creation. Updated manually on new messages. |

---

## `messages`

The core table — one row per email.

| Field | Type | Notes |
|-------|------|-------|
| `id` | `INTEGER PRIMARY KEY` | **Critical** — this exact value is synced to `messages_fts.rowid` for fast joins. |
| `account_id` | `INTEGER NOT NULL REFERENCES accounts(id)` | Which account received this message. |
| `mailbox_id` | `INTEGER NOT NULL REFERENCES mailboxes(id)` | Which folder it's in. |
| `uid` | `INTEGER NOT NULL` | IMAP UID — unique within a mailbox + uid_validity pair. |
| `modseq` | `INTEGER` | IMAP modification sequence — detects if message was changed server-side. |
| `message_id` | `TEXT UNIQUE` | RFC 2822 Message-ID header (e.g. `<abc123@gmail.com>`). Can rarely collide. |
| `thread_id` | `INTEGER REFERENCES threads(id) ON DELETE SET NULL` | Which conversation this belongs to. Set to NULL if thread is deleted. |
| `subject` | `TEXT` | Email subject line. Indexed by FTS5. |
| `from_name` | `TEXT` | Display name of sender. |
| `from_address` | `TEXT` | Email address of sender. |
| `date_sent` | `DATETIME` | When the email was sent (from headers). |
| `date_received` | `DATETIME` | When we received it (may differ from sent). |
| `file_path` | `TEXT NOT NULL` | Path to the raw `.eml` file on disk. |
| `size` | `INTEGER` | File size in bytes. |
| `body_text` | `TEXT` | Clean plain-text body. Indexed by FTS5. |
| `body_html` | `TEXT` | Raw HTML body (optional, for rendering). |
| `flags` | `INTEGER DEFAULT 0` | Bitmask: `1=Seen`, `2=Answered`, `4=Flagged`, `8=Deleted`, `16=Draft`. |
| `custom_flags` | `TEXT` | JSON for IMAP keywords like `\Seen`, `$label`, etc. |
| `created_at` | `DATETIME DEFAULT CURRENT_TIMESTAMP` | When we stored it locally. |
| `UNIQUE(mailbox_id, uid)` | Composite constraint | Prevents duplicate messages in the same folder. |

---

## `messages_fts` (FTS5 Virtual Table)

Full-text search index. Not a real table — it's a virtual table managed by SQLite's FTS5 extension.

| Field | Type | Notes |
|-------|------|-------|
| `rowid` | Hidden | **Must match** `messages.id` exactly. This is how we JOIN without text lookups. |
| `subject` | `TEXT` | Indexed for search. Queried with `MATCH`. |
| `body_text` | `TEXT` | Indexed for search. Queried with `MATCH`. |
| `content='messages'` | Config | Tells FTS5 this is a shadow of the `messages` table. |
| `content_rowid='id'` | Config | Tells FTS5 to use `messages.id` as the rowid. |
| `tokenize='porter unicode61'` | Config | Porter stemmer (run→running→ran) + unicode-aware word splitting. |

**Example query:**
```sql
SELECT m.id, m.subject, snippet(messages_fts, -1, '<b>', '</b>', '...', 64)
FROM messages_fts f
JOIN messages m ON m.id = f.rowid
WHERE f MATCH 'invoice'
ORDER BY rank;
```

---

## `message_recipients`

Denormalized recipient table for fast "emails from/to X" queries.

| Field | Type | Notes |
|-------|------|-------|
| `message_id` | `INTEGER REFERENCES messages(id) ON DELETE CASCADE` | Which email this recipient is on. |
| `type` | `TEXT CHECK(...)` | Constrained to `'from'`, `'to'`, `'cc'`, or `'bcc'`. |
| `name` | `TEXT` | Display name. |
| `address` | `TEXT NOT NULL` | Email address. |
| `PRIMARY KEY (message_id, type, address)` | Composite key | Prevents duplicate recipient entries per message. |

---

## `attachments`

Metadata for files attached to emails.

| Field | Type | Notes |
|-------|------|-------|
| `id` | `INTEGER PRIMARY KEY` | Auto-increment rowid. |
| `message_id` | `INTEGER REFERENCES messages(id) ON DELETE CASCADE` | Which email this attachment belongs to. |
| `filename` | `TEXT` | Original filename. |
| `content_type` | `TEXT` | MIME type (e.g. `image/png`, `application/pdf`). |
| `size` | `INTEGER` | File size in bytes. |
| `file_path` | `TEXT` | Path to the extracted attachment on disk. |
| `cid` | `TEXT` | Content-ID for inline images (referenced in HTML via `cid:...`). |

---

## Triggers

Three triggers keep `messages_fts` in sync with `messages` automatically.

### `messages_ai` (After Insert)
```sql
CREATE TRIGGER messages_ai AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts(rowid, subject, body_text)
    VALUES (new.id, new.subject, new.body_text);
END;
```
When a new message is inserted, this copies its `id`, `subject`, and `body_text` into the FTS index.

### `messages_ad` (After Delete)
```sql
CREATE TRIGGER messages_ad AFTER DELETE ON messages BEGIN
    DELETE FROM messages_fts WHERE rowid = old.id;
END;
```
When a message is deleted, this removes its FTS entry.

### `messages_au` (After Update)
```sql
CREATE TRIGGER messages_au AFTER UPDATE ON messages BEGIN
    UPDATE messages_fts
    SET subject = new.subject, body_text = new.body_text
    WHERE rowid = old.id;
END;
```
When a message is updated (e.g. body parsed later), this updates the FTS entry.

**Key idea:** `new` and `old` are special pseudo-tables available inside triggers that reference the row being modified.
