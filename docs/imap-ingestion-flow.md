# IMAP Ingestion Pipeline

This document describes how the program moves mail from the IMAP server into the local SQLite database.

## Visual Diagram

```text
┌─────────────┐      ┌─────────────┐      ┌─────────────┐      ┌─────────────┐
│ IMAP Server │      │   Puffin    │      │ Local Files │      │  SQLite DB  │
└──────┬──────┘      └──────┬──────┘      └──────┬──────┘      └──────┬──────┘
       │                    │                    │                    │
       │ 0. STARTTLS + LOGIN│                    │                    │
       │ requires CONDSTORE,│                    │                    │
       │ and IDLE caps      │                    │                    │
       │◄───────────────────│                    │                    │
       │                    │                    │                    │
       │ 1. SELECT mailbox  │                    │                    │
       │    with CONDSTORE  │                    │                    │
       │◄───────────────────│                    │                    │
       │                    │                    │                    │
       │ 2. UID FETCH new   │                    │                    │
       │    UIDs, send:     │                    │                    │
       │    ENVELOPE, FLAGS,│                    │                    │
       │    BODY[] with PEEK│                    │                    │
       │◄───────────────────│                    │                    │
       │      (Raw .eml)    │                    │                    │
       │───────────────────►│                    │                    │
       │                    │                    │                    │
       │                    │ 3. Write .eml copy │                    │
       │                    │    userdata/       │                    │
       │                    │    <name>_<id>/    │                    │
       │                    │    <uid>.eml       │                    │
       │                    │───────────────────►│                    │
       │                    │                    │                    │
       │                    │ 4. Parse raw bytes │                    │
       │                    │    already in RAM, │                    │
       │                    │    not from disk.  │                    │
       │                    │                    │                    │
       │                    │                    │ 5. INSERT message  │
       │                    │                    │    row             │
       │                    │                    │───────────────────►│
       │                    │                    │                    │
       │ 6. UID SEARCH ALL  │                    │                    │
       │    reconcile pass, │                    │                    │
       │    runs when local │                    │                    │
       │    MODSEQ is behind│                    │                    │
       │◄───────────────────│                    │                    │
       │                    │                    │                    │
       │                    │ 7. rm .eml files   │                    │
       │                    │    whose UID is not│                    │
       │                    │    listed on server│                    │
       │                    │───────────────────►│                    │
       │                    │                    │ 7. DELETE message  │
       │                    │                    │    rows whose UIDs │
       │                    │                    │    the server lacks│
       │                    │                    │───────────────────►│
       │                    │                    │                    │
       │ 8. UID FETCH FLAGS │                    │                    │
       │    changed since   │                    │                    │
       │    stored MODSEQ   │                    │                    │
       │    (CHANGEDSINCE)  │                    │                    │
       │◄───────────────────│                    │                    │
       │                    │                    │                    │
       │                    │                    │ 9. UPDATE flags and│
       │                    │                    │    modseq          │
       │                    │                    │───────────────────►│
       │                    │                    │                    │
       │                    │                    │ 10. UPDATE mailbox │
       │                    │                    │     sync state:    │
       │                    │                    │     uid_next       │
       │                    │                    │     uid_validity   │
       │                    │                    │     highest_modseq │
       │                    │                    │     last_sync      │
       │                    │                    │───────────────────►│
       │                    │                    │                    │
       │ 11. IDLE           │                    │                    │
       │◄───────────────────│                    │                    │
       │     EXISTS/EXPUNGE/│                    │                    │
       │     FETCH events   │                    │                    │
       │───────────────────►│                    │                    │
       │                    │                    │                    │
```

## Step notes

- Step 0. Connect. The program dials the server with StartTLS and logs in. The connection stops here when the server does not offer `CONDSTORE` or `IDLE`.
- Step 1. Open the mailbox. The sync runs `SELECT` with `CONDSTORE`. The server sends UIDVALIDITY, UIDNEXT, and HIGHESTMODSEQ. These three values are the sync state of the mailbox.
- Step 2. Fetch new messages. The fetch range starts at the first UID that the local database does not have. It ends one UID below the server UIDNEXT. The server assigns new UIDs from this value, so a range that ends with `:*` can include messages that arrive during the fetch.
- Step 4. Parse. The fetch response holds the raw bytes in memory. The parser reads these bytes. It does not read the file from disk. It stores the text of the plain-text parts and the HTML parts in the `message.body` column. The code removes the tags from the HTML parts. The `.eml` file is a copy for later use. A later read, for example when a message is opened, loads this file from disk.
- Step 6. Reconcile. This pass runs only when the stored HIGHESTMODSEQ is not zero. The pass records all UIDs on the server. It deletes every local row and `.eml` file with a UID that the server does not list.
- Step 8. Update flags. The fetch runs with `CHANGEDSINCE <modseq>` over the full UID range. The code writes the new values into `flags` and `modseq`.
- Step 11. IDLE. The watcher waits in IDLE. Unilateral data is server status that arrives while IDLE is active. The loop wakes on three events: the server sends unilateral data, the wake-up channel receives an event, or the 29-minute refresh timer runs out. The loop then starts at step 1 again.
