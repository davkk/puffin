# CONDSTORE, MODSEQ, and CHANGEDSINCE

This page explains two IMAP counters and one fetch modifier. Each picture shows one idea. The values come from the CONDSTORE extension, RFC 7162.

## Two counters in one mailbox

A mailbox numbers its messages with UIDs. It numbers its changes with MODSEQ. A new epoch is a new UIDVALIDITY value, and a new epoch resets both counters.

```text
UIDVALIDITY = 7            the epoch, a new epoch resets the UIDs

UID counter                append only inside one epoch
  1  2  3  4  5  6  7
  │  │  │  │  │  │  │
  first                  UIDNEXT = 8 (the next free UID)

MODSEQ counter             monotone, one per mailbox
  104  105  106
             ▲
             HIGHESTMODSEQ = 106 (the top)
```

UIDNEXT is the next free UID. HIGHESTMODSEQ is the top of the change counter.

## What bumps MODSEQ

Arrivals, expunges, and flag changes bump the counter. Each affected message receives the new value.

```text
        msg5       msg2       msg3       msg5
      arrives      \Seen    expunged   \Deleted
         │          │          │          │
  ───────┼──────────┼──────────┼──────────┼────► time
        104        105        106        107

  the counter only goes up
```

## The baseline and the window

A client stores HIGHESTMODSEQ after each sync. That stored value is the baseline. CHANGEDSINCE returns only the messages above the baseline.

```text
  fetch:  UID FETCH 1:* (FLAGS) (CHANGEDSINCE 104)

  baseline 104
      │
      │  MODSEQ 104 or less    ->  silent, not returned
      │
      └─>  MODSEQ above 104    ->  returned with new flags,
                                   and a new HIGHESTMODSEQ
```

## Why the extension exists

Without the extension, every sync asks for every flag. The work grows with the size of the mailbox.

```text
  no CONDSTORE  FETCH 1:* FLAGS     ████████████████████
  CONDSTORE     FETCH CHANGEDSINCE  ░░

  without CONDSTORE, the work grows with the mailbox
  with CONDSTORE, the work grows with the changes
```

## The decision points in this code

Each row is a comparison that the sync performs.

```text
  server value     local state       what to do
  UIDNEXT          smaller           fetch the new UID range
  HIGHESTMODSEQ    smaller           run the CHANGEDSINCE pass
  UIDVALIDITY      different         wipe the mailbox, start over
  all equal        nothing           go straight to IDLE
```

## Where each value lives

The protocol values, the schema columns, and the actions line up one to one.

```text
  protocol         schema                   action
  UIDVALIDITY      mailbox.uid_validity     reset the mailbox when it changes
  UIDNEXT          mailbox.uid_next         fetch the UIDs above the stored value
  HIGHESTMODSEQ    mailbox.highest_modseq   pass it as CHANGEDSINCE
  per-msg MODSEQ   message.modseq           store it on each changed message
```

## What QRESYNC adds

CHANGEDSINCE does not report deleted messages. QRESYNC reports them as VANISHED.

```text
  CHANGEDSINCE  ->  messages with new flags
  QRESYNC       ->  the UIDs that vanished
```

With QRESYNC, the full UID SEARCH pass that the reconcile step performs becomes unnecessary.
