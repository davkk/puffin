# IMAP Ingestion Pipeline

This document describes the flow for fetching data from the IMAP server and storing it in the local SQLite database.

## Visual Diagram

```text
┌──────────────┐      ┌──────────────┐      ┌──────────────┐
│  IMAP Server │      │  File System │      │   SQLite DB  │
└──────┬───────┘      └──────┬───────┘      └──────┬───────┘
       │                     │                     │
       │ 1. SELECT INBOX     │                     │
       │    (CONDSTORE)      │                     │
       │◄────────────────────┤                     │
       │    UIDVALIDITY      │                     │
       │    UIDNEXT, MODSEQ  │                     │
       │                     │                     │
       │ 2. UID FETCH        │                     │
       │    <uid_next>:*     │                     │
       │    (ENVELOPE FLAGS  │                     │
       │     BODY[])         │                     │
       │────────────────────►│                     │
       │    (Raw .eml)       │ 3. Write            │
       │                     │    userdata/        │
       │                     │    <name>_<id>/     │
       │                     │    <uid>.eml        │
       │                     │                     │
       │                     │ 4. Parse .eml       │
       │                     │    (headers, body)  │
       │                     │                     │
       │                     │ 5. INSERT message   │
       │                     │────────────────────►│
       │                     │                     │
       │                     │ 6. Trigger fires    │
       │                     │    (message_ai →    │
       │                     │     message_fts)    │
       │                     │                     │
       │ 7. UID SEARCH ALL   │                     │
       │    (deletion pass)  │                     │
       │◄────────────────────┤                     │
       │────────────────────►│ 8. DELETE local     │
       │                     │    rows not on      │
       │                     │    server + rm .eml │
       │                     │                     │
       │ 9. UID FETCH 1:*    │                     │
       │    (FLAGS)          │                     │
       │    CHANGEDSINCE     │                     │
       │    <modseq>         │                     │
       │◄────────────────────┤                     │
       │────────────────────►│ 10. UPDATE flags,   │
       │                     │     modseq          │
       │                     │                     │
       │                     │ 11. UPDATE mailbox  │
       │                     │     (uid_next,      │
       │                     │      uid_validity,  │
       │                     │      highest_modseq,│
       │                     │      last_sync)     │
       │                     │                     │
       │ 12. IDLE            │                     │
       │◄────────────────────┤                     │
       │    EXISTS/EXPUNGE/  │                     │
       │    FETCH → repeat   │                     │
```
