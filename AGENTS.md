# Puffin — Obsidian-Style Email Client

## Vision

A fast, native email client for power users who prefer Markdown and Vim. It combines the speed of a local SQLite-backed C/Go application with the flexibility of your own terminal-based `$EDITOR`.

Core pillars: zero lag, true offline-first capability, no telemetry, and a markdown-first workflow.

## Language & Stack

**One language: Go.**.

| Layer | Choice | Rationale |
|-------|--------|-----------|
| UI Framework | **gotk4** | Native GTK4 performance and styling |
| WebKitGTK | **gotk4-webkitgtk** | Secure HTML rendering for email bodies |
| Fuzzy Search | `github.com/sahilm/fuzzy` | Lightning-fast "Omnibar" navigation |
| IMAP/SMTP | `go-imap/v2`, `go-smtp` | Standard, reliable Go mail protocols |
| Database | `modernc.org/sqlite` | Pure Go SQLite for local-first storage and FTS5 |
| Markdown | `gomarkdown` | Renders your drafts into clean HTML |

## Architecture: Fast Search & Navigation

Instead of complex linking, Puffin uses a two-tier search system to help you find anything in milliseconds:

1. **The Fuzzy Omnibar (`Ctrl+P` or `/`):**
   - Keeps a lightweight index of Subjects and Contacts in memory.
   - Provides instant, fuzzy filtering as you type (similar to `fzf`).
   - The primary way to jump between threads without touching the mouse.
2. **Deep Content Search (SQLite FTS5):**
   - Indexes the full body of every email stored locally.
   - Used for heavy-duty queries where you need to find specific text across years of archives.

## Architecture: Hybrid Composer Flow

Puffin uses a `GtkTextView` for immediate typing, but lets you "eject" to a real terminal for complex emails.

```text
┌────────────────────────────────────────────────────────┐
│ Compose Window                                         │
│  To: [_____________]  Subject: [___________________]   │
│ ├────────────────────────┬───────────────────────────┤ │
│ │ GtkTextView (Native)   │ WebKitGTK (Live Preview)  │ │
│ │                        │                           │ │
│ │ # Hello                │ <h1>Hello</h1>            │ │
│ │                        │                           │ │
│ │ Typing here is fast.   │ <p>Typing here is fast.</p>│ │
│ │                        │                           │ │
│ ├────────────────────────┴───────────────────────────┤ │
│ │ [Send]  [Save Draft]   [Open in $EDITOR (Ctrl+E)]  │ │
└─┴────────────────────────────────────────────────────┴─┘
```

### The $EDITOR Handoff

1. **Trigger:** User hits `Ctrl+E`.
2. **Transfer:** Puffin saves the current text to `/tmp/puffin-draft-$uuid.md`.
3. **Launch:** Puffin spawns a terminal (e.g., `x-terminal-emulator -e nvim`) on that file.
4. **Lock:** The native GUI text box is disabled to prevent "split-brain" edits.
5. **Sync:** When the editor closes, Puffin pulls the new text back into the GUI and unlocks it.

## Vim Keybindings (Navigation Mode)

Implemented in `internal/ui/vim_nav.go`:

| Key | Action |
| --- | --- |
| `j` / `k` | Move selection down/up |
| `o` / `Enter` | Open/Expand thread |
| `Esc` | Return to list / Clear search |
| `/` | Open Fuzzy Omnibar |
| `c` | New Draft |
| `r` | Reply |
| `d` | Archive/Delete |

## Data Model (SQLite)

```sql
emails: id TEXT PK, msg_id TEXT, thread_id TEXT, subject TEXT,
        from_addr TEXT, to_addr TEXT, cc TEXT, body_text TEXT,
        body_html TEXT, date INTEGER, synced_at INTEGER

drafts: id TEXT PK, to_addr TEXT, cc TEXT, subject TEXT,
        body_path TEXT, created_at INTEGER
```

## Implementation Phases

1. **Phase 1: The Shell.** Build the GTK4 layout with the native text box and WebKit preview.
2. **Phase 2: Local Storage.** Set up SQLite and the metadata fuzzy index.
3. **Phase 4: IMAP Sync.** Implement background syncing and IDLE push.
4. **Phase 4: The Handoff.** Implement terminal spawning and file-watch for `$EDITOR`.
5. **Phase 5: Search.** Finalize FTS5 deep search and UI polish.

---

# email receive flow

This diagram provides a high-level overview of the entire hybrid storage and
indexing architecture. Here is a breakdown of the four key stages in the data
lifecycle:

### Stage 1: Ingestion and Raw Storage

The process begins when an incoming email arrives. The architecture splits
immediately to achieve the hybrid storage goals:

* **(1)** The raw, unmodified `.eml` file—including all its multipart
  boundaries, HTML formatting, and attachments—is saved directly to the host
  **File System (OS)**.
* The raw file is simultaneously passed to the SQLite processing engine to
  populate the metadata and search indices.

### Stage 2: Parsing and Threading Logic

Within SQLite, the email is decomposed:

* **(2a) Parsing Logic:** The client extracts critical headers needed for
  display and threading (`Message-ID`, `References`, subject, date). Crucially,
  this stage also identifies and extracts the primary content body:
* It prefers `text/plain` for simplicity.
* If only `text/html` exists, it **must strip the HTML tags** to generate clean
  plain text. This prevents FTS5 from indexing formatting code like `<div>` and
  `href`, ensuring high-quality search results.


* **(2b) Threading Logic:** The headers (`Message-ID`, `In-Reply-To`) are
  analyzed to resolve the correct `thread_id`. The logic looks up existing
  threads. If the email is part of a known conversation, it retrieves the ID;
  otherwise, it creates a new entry in the `THREADS TABLE`.

### Stage 3: Database Insertion and FTS5 Sync

This is the most critical technical step for high-performance indexing:

* **(3a) Metadata Insertion:** The parsed headers (subject, dates), the
  resolved `thread_id`, and the pointer to the file on disk (`file_path`) are
  inserted into the standard relational **MESSAGES TABLE**. This table
  generates a unique Integer primary key (`id`).
* **The Sync (CRITICAL):** The system captures the unique Integer `id`
  generated in Step 3a.
* **(3b) Search Data Insertion:** The cleaned plain-text body and subject are
  inserted into the **MESSAGES_FTS TABLE**, *forcing* the use of the exact same
  Integer captured in Step 3a as the virtual table's `rowid`. This synchronizes
  both tables without needing slow text-based UUID joins.

### Stage 4: Query Flow

When a user performs a search:

* **(4)** The **SEARCH QUERY** hits the `messages_fts` table using the
  efficient `MATCH` operation. Because the tables share synchronized Integer
  IDs, the query can instantly `JOIN` the `messages_fts` table with the
  `messages` table to present the user with relevant results, complete with
  thread context and hit-highlighted text snippets.

---

# database schema

```sql
-- 1. Accounts (multiple email accounts support)
CREATE TABLE accounts (
    id INTEGER PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    imap_host TEXT NOT NULL,
    imap_port INTEGER,
    username TEXT NOT NULL,
    -- encrypted password or token
    password_encrypted BLOB,
    sync_enabled BOOLEAN DEFAULT 1,
    last_sync DATETIME
);

-- 2. Mailboxes / Folders
CREATE TABLE mailboxes (
    id INTEGER PRIMARY KEY,
    account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    name TEXT NOT NULL,                    -- e.g. "INBOX", "Sent", "Archive"
    path TEXT,                             -- IMAP folder path
    uid_validity INTEGER,                  -- Critical for IMAP sync
    uid_next INTEGER,
    highest_modseq INTEGER,                -- For CONDSTORE/QRESYNC
    last_sync DATETIME,
    UNIQUE(account_id, name)
);

-- 3. Threads
CREATE TABLE threads (
    id INTEGER PRIMARY KEY,
    account_id INTEGER NOT NULL REFERENCES accounts(id),

    thread_key TEXT UNIQUE NOT NULL,           -- e.g. Gmail X-GM-THRID or computed root Message-ID
    subject_normalized TEXT,                   -- "Re: Hello" → "Hello"

    first_message_id INTEGER,                  -- REFERENCES messages(id)
    last_message_date DATETIME,
    message_count INTEGER DEFAULT 0,
    unread_count INTEGER DEFAULT 0,

    participants TEXT,                         -- JSON array of {name, address} for preview
    snippet TEXT,                              -- Last message snippet

    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- In messages table
ALTER TABLE messages ADD COLUMN thread_id INTEGER REFERENCES threads(id) ON DELETE SET NULL;

-- 4. Messages (Core table)
CREATE TABLE messages (
    id INTEGER PRIMARY KEY,                -- Used for FTS rowid sync

    account_id INTEGER NOT NULL REFERENCES accounts(id),
    mailbox_id INTEGER NOT NULL REFERENCES mailboxes(id),

    -- IMAP identifiers
    uid INTEGER NOT NULL,                  -- IMAP UID
    modseq INTEGER,                        -- For change detection

    message_id TEXT UNIQUE,                -- RFC Message-ID (can collide rarely)

    thread_id INTEGER REFERENCES threads(id) ON DELETE SET NULL,

    subject TEXT,
    from_name TEXT,
    from_address TEXT,
    date_sent DATETIME,
    date_received DATETIME,

    file_path TEXT NOT NULL,               -- Path to raw .eml
    size INTEGER,

    body_text TEXT,                        -- Clean plain text for FTS + previews
    body_html TEXT,                        -- Optional

    flags INTEGER DEFAULT 0,               -- Bitmask: Seen, Answered, Flagged, Deleted, Draft...
    custom_flags TEXT,                     -- JSON for IMAP keywords (\Seen, $label, etc.)

    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(mailbox_id, uid)                -- Important for sync
);

-- 5. FTS5 (exactly as you wanted)
CREATE VIRTUAL TABLE messages_fts USING fts5(
    subject,
    body_text,
    content='messages',
    content_rowid='id',
    tokenize='porter unicode61'
);

-- 6. Recipients & Attachments
CREATE TABLE message_recipients (
    message_id INTEGER REFERENCES messages(id) ON DELETE CASCADE,
    type TEXT CHECK(type IN ('from', 'to', 'cc', 'bcc')),  -- 'from' is denormalized for convenience
    name TEXT,
    address TEXT NOT NULL,
    PRIMARY KEY (message_id, type, address)
);

CREATE TABLE attachments (
    id INTEGER PRIMARY KEY,
    message_id INTEGER REFERENCES messages(id) ON DELETE CASCADE,
    filename TEXT,
    content_type TEXT,
    size INTEGER,
    file_path TEXT,                    -- extracted attachment on disk
    cid TEXT                           -- Content-ID for inline images
);

```

```sql
----- INSERTION

-- Pseudo-code for ingestion
BEGIN TRANSACTION;

-- Insert or get thread
INSERT OR IGNORE INTO threads (thread_key, subject_normalized, first_message_date, ...)
VALUES (?, ?, ?, ...);

-- Get thread_id
SELECT id INTO :thread_id FROM threads WHERE thread_key = ?;

-- Insert message (gets auto-increment id)
INSERT INTO messages (thread_id, message_id, subject, from_address, date_received,
                      file_path, body_text, body_html, ...)
VALUES (:thread_id, :msg_id, :subject, ..., :eml_path, :clean_text, :html, ...);

-- Capture the exact rowid we just got
SET :new_id = last_insert_rowid();

-- Sync FTS (this is the critical part)
INSERT INTO messages_fts (rowid, subject, body_text)
VALUES (:new_id, :subject, :clean_text);

COMMIT;
```

lots of room for extra sqlite triggers

---

# storing the emails

+--------------------------------+-------------------------------------+-------------------------------+
| Feature                        | mbox (Thunderbird style)            | One .eml per message          |
+--------------------------------+-------------------------------------+-------------------------------+
| Storage Format                 | Single large file per mailbox       | Individual .eml files         |
| File Count                     | 1 file per folder                   | 1 file per email              |
| Robustness                     | Low (one corruption can lose many)  | High (only one email affected)|
| Concurrency                    | Poor (file locking issues)          | Excellent                     |
| Incremental Backup             | Bad                                 | Excellent                     |
| Disk Usage Efficiency          | Better                              | Slightly worse (overhead)     |
| Performance (small mailbox)    | Very Fast                           | Fast                          |
| Performance (large mailbox)    | Slows down significantly            | Good (with subfolders)        |
| Deleting / Modifying one email | Expensive (rewrite whole file)      | Cheap & Fast                  |
| IMAP Two-way Sync              | Good                                | Better                        |
| Debugging / Manual Inspection  | Difficult                           | Very Easy                     |
| Exporting single email/thread  | Hard                                | Very Easy                     |
| OS Filesystem Limits           | No issue                            | Need to avoid huge folders    |
| Implementation Complexity      | Simple                              | Medium                        |
| Recommended For                | Very lightweight clients            | Modern full-featured clients  |
+--------------------------------+-------------------------------------+-------------------------------+

```
~/.config/yourclient/               # or ~/.local/share/yourclient/
    data/
        accounts/
            1_john_doe_gmail_com/
                mailboxes/
                    INBOX/
                        2026/
                            05/
                                145823.eml
                                145824_abc123@google.com.eml
                        2026/
                            04/
                    Sent/
                    Archive/
                attachments/
```

---

### Key Stages Explained

1. **Phase 1 (The Handshake):** Crucial setup. Your application logic (`Main`) is separated conceptually from the library's internal, long-lived background listener (`Lib`). You **must** attach the `UnilateralDataHandler` before dialing or enabling extensions; otherwise, you will miss the server's immediate broadcast during the `SELECT` phase.
2. **Phase 2 (The QRESYNC Select):** This is the magic.
* **Step 14:** You query SQLite for **EVERY SINGLE UID** you currently possess for this mailbox.
* **Step 16:** You pack your local `ModSeq`, `Validity`, and *that list of UIDs* into the `SelectOptions`.
* **Step 17:** The server receives this and immediately compares your Known UIDs list to its current database. It calculates what you have that *it doesn't* have (i.e., things deleted elsewhere while you were offline).
* **Steps 19-25:** The server *immediately* starts firing `VANISHED` responses into the TCP stream *while your synchronous `Select()` call is still blocked and waiting*. Because you hooked the `UnilateralDataHandler` in Phase 1, the library's background thread catches these broadcasts and passes them to your application for deletion in the DB concurrently.
* **Step 26:** The server finally says "OK SELECT completed" and returns the updated `HIGHESTMODSEQ` and `UIDNEXT`.


3. **Phase 3 (Active Sync):** The historical deletions are handled. Now you handle flag changes and new mail.
* **Flag Diffs (CONDSTORE):** You compare your `local ModSeq` (5000) against the new `Server HighModSeq` (6000). Since 6000 > 5000, you send a highly efficient `FETCH ... CHANGEDSINCE 5000`. The server returns *only* those messages modified since your last sync. (This catches flag changes `CONDSTORE` missed during the implicit catch-up).
* **New Mail (UIDNext):** This is the same standard sync you had in your PoC, fetching anything between your database's `uid_next` and the server's reported `uid_next`.


4. **Phase 4 (Finalize State):** You update the mailbox state table in SQLite. It is absolutely vital to save the *new* `highest_modseq` (6000) so the next sync knows exactly where to begin.
