# Sync strategies in IMAP clients

This page describes how IMAP clients avoid a full listing on every sync. It uses the same terms as `docs/imap-concepts.md`. A delta is the difference between the server state and the client state.

## The four rungs

Most clients ask for a delta and almost never for a full listing. Each rung turns on one server capability.

```text
  what the client stores         the server answers               cost
  UIDVALIDITY + UIDNEXT          the new UIDs only                one UID FETCH
  plus HIGHESTMODSEQ             the changed flags only           one UID FETCH
  plus known UIDs (QRESYNC)      vanished, changed, and new       one SELECT
  plus an open IDLE connection   each event as it happens         nothing while alive
```

The bottom rung works on every server, because RFC 3501 defines it. The client stores the mailbox UIDVALIDITY and the highest UID that it holds. On sync it selects the mailbox and compares the server UIDNEXT with the stored value. Only the UIDs above the stored value are new, so the client fetches that range. This rung detects arrivals and nothing else. Flag changes and deletions still force a full `UID FETCH 1:* (FLAGS)` pass. That pass is the cost that clients mean when they speak of a full fetch.

The second rung adds CONDSTORE. The client stores HIGHESTMODSEQ and sends `CHANGEDSINCE <baseline>`, so the flag listing shrinks to the messages that changed.

The third rung adds QRESYNC. The client sends its known UIDs and its stored MODSEQ inside the SELECT command. The server answers with three sets in one round trip: the VANISHED messages, the changed flags, and the new arrivals. This rung removes the UID SEARCH diff that the reconcile pass performs.

The fourth rung is IDLE, which is a push channel and not a resync. The client holds one connection per account and receives EXISTS, EXPUNGE, and FETCH events as they arrive, so in steady state it asks for nothing. The three rungs below it still matter, because they cover a restart, a crash, and a dropped connection.

## Keep the body lazy

The heavy part of a message is the body. Mature clients fetch it on demand.

```text
  sync time           ENVELOPE + FLAGS + UID  (small, for every new message)
  open time           BODY[] for one message  (large, on demand)
```

A client therefore fetches ENVELOPE, FLAGS, and UID for every new message, and it fetches `BODY[]` only when the user opens the message. Some clients also fetch BODYSTRUCTURE during sync for the attachment icon and the preview text.

## Remember the state on disk

A client writes the sync state to disk after every pass. The state is small: UIDVALIDITY, UIDNEXT, HIGHESTMODSEQ, and the highest UID that the client holds. A restart then reloads that state, so a mailbox without changes costs one SELECT and no fetch.

## Servers without CONDSTORE

Two habits cover a server that offers neither CONDSTORE nor QRESYNC. The client lists flags only for the folders that the user opens, and not for every folder at startup. The client also keeps an array of UIDs indexed by sequence number.

```text
  UID by sequence:   [57] [58] [59] [60]
  the server expunges UID 58
  UID by sequence:   [57] [59] [60]
  the array shifts, so no new UID listing is needed
```

Thunderbird uses this array and calls it the backfill array. It removes the need for a repeated full listing after an expunge.

## Message identity

A UID identifies a message inside one mailbox epoch. A mailbox rename, a move between folders, or a UIDVALIDITY reset breaks that identity. Clients use a second identity for those cases. OBJECTID (RFC 8474) and the Gmail value X-GM-MSGID survive a move and a rename. The Message-ID header together with the MOVE extension gives a similar result on servers without OBJECTID.

## Which providers offer what

```text
  provider         CONDSTORE   QRESYNC
  Dovecot          yes         yes
  Cyrus, Fastmail  yes         yes
  Zimbra           yes         yes
  iCloud           yes         yes
  Gmail            yes         no
  Exchange Online  no          no
  Yahoo            no          no
```

The table comes from the Thunderbird QRESYNC work in Mozilla bug 1747311, together with capability dumps from Microsoft support threads. The values can change, so read the CAPABILITY answer from the server itself. The Dovecot test container for this project answers with CONDSTORE and QRESYNC after login, so the fast path stays testable.

## What Puffin does today

Three facts separate the current code from the ladder.

- The new-message fetch requests `BODY[]` with Peek inside the sync path, so every new message costs its full body during sync.
- The insert of a new message leaves the `modseq` column at zero, so the CHANGEDSINCE baseline does not cover the new rows.
- The connection requires CONDSTORE and IDLE, and a missing capability stops the program with a fatal log. Puffin therefore refuses to start against Exchange Online, Yahoo, and ForwardEmail.

Three changes close most of the gap: fetch the body when the user opens the message, store MODSEQ during the insert, and choose the CHANGEDSINCE or QRESYNC path only when the capability answer offers it.
