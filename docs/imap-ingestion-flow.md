# IMAP Ingestion Pipeline

This document describes the flow for fetching data from the IMAP server and storing it in the local SQLite database.

## High-Level Flow

### 1. Handshake & Validation
*   Connect to IMAP.
*   `SELECT` the mailbox (e.g., "INBOX").
*   **Check UIDVALIDITY**: Compare server's UIDVALIDITY with local `mailbox.uid_validity`.
    *   If they differ, the server reset. You must **wipe local messages** for this mailbox and start over.

### 2. Identify New Messages
*   Query local DB: `SELECT MAX(uid) FROM message WHERE mailbox_id = ?`
*   Ask IMAP: `SEARCH UID SINCE <local_max_uid + 1>` (or fetch range).
*   *Result:* A list of UIDs to download.

### 3. Fetch & Save (The "Hybrid" Step)
For each new UID:
1.  **FETCH BODY[]**: Download the full raw `.eml` content.
2.  **Save to Disk**: Write to `data/accounts/<id>/mailboxes/<name>/<uid>.eml`.
3.  **Parse**: Read the file to extract headers (`Message-ID`, `In-Reply-To`, `Subject`) and body text.

### 4. Resolve Threading
*   Look at `In-Reply-To` and `References` headers.
*   Query DB: `SELECT thread_id FROM message WHERE message_id IN (...)`
*   **Found parent?** Use parent's `thread_id`.
*   **No parent?** Create a new row in `thread` table and get the new ID.

### 5. Commit to Database
*   **Insert Message**:
    ```sql
    INSERT INTO message (mailbox_id, uid, thread_id, subject, from_address, date_sent, file_path, body)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    ```
*   **Get RowID**: `last_insert_rowid()` (This is critical for FTS5 sync).
*   **Update Mailbox**: `UPDATE mailbox SET uid_next = ? WHERE id = ?`

## Visual Diagram

```text
┌──────────────┐      ┌──────────────┐      ┌──────────────┐
│  IMAP Server │      │  File System │      │   SQLite DB  │
└──────┬───────┘      └──────┬───────┘      └──────┬───────┘
       │                     │                     │
       │ 1. SELECT INBOX     │                     │
       │ 2. UID SEARCH ALL   │                     │
       │◄────────────────────┤                     │
       │ (List of UIDs)      │                     │
       │                     │                     │
       │ 3. FETCH UID 123    │                     │
       │ (Raw .eml)          │                     │
       │────────────────────►│                     │
       │                     │ 4. Parse .eml       │
       │                     │ (Headers, Body)     │
       │                     │                     │
       │                     │ 5. Check Thread     │
       │                     │ (References header) │
       │                     │────────────────────►│
       │                     │◄────────────────────┤
       │                     │ (Thread ID)         │
       │                     │                     │
       │                     │ 6. INSERT message   │
       │                     │────────────────────►│
       │                     │                     │
       │                     │ 7. Trigger fires    │
       │                     │ (Sync FTS5)         │
       │                     │                     │
```

## Why this order?

1.  **Save File First:** If the DB insert fails, you still have the raw email on disk. You can re-parse it later. If you inserted into DB first and failed to save the file, you'd have a "ghost" message with no content.
2.  **Thread Resolution:** You need the parsed headers (specifically `References`) to figure out the `thread_id` *before* you insert the message row.
