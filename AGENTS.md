# Puffin

A fast, offline-first email client.

## Language & Stack

IMAP/SMTP - `go-imap/v2`, `go-smtp`
Database - `modernc.org/sqlite`

## Storing the emails

```
~/.local/share/puffin/
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
