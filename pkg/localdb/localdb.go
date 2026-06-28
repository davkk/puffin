package localdb

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"puffin/pkg/assert"
	"strings"
	"time"
)

//go:embed schema.sql
var sqlSchema string

func ConnectSqlite(filepath string) (*sql.DB, error) {
	assert.Assert(filepath != "", "empty filepath")

	db, err := sql.Open("sqlite", filepath)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err = db.Exec(sqlSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	return db, nil
}

func Search(db *sql.DB, query string) ([]MessageEntry, error) {
	rows, err := db.Query(`
		SELECT
			message_fts.rowid AS id,
			highlight(message_fts, 0, '<b>', '</b>') AS subject_highlighted,
			snippet(message_fts, 1, '<b>', '</b>', '', 100) AS body_snippet,
			m.from_name,
			m.from_address,
			m.date
		FROM message_fts
		JOIN message m ON m.id = message_fts.rowid
		WHERE message_fts MATCH ?;
	`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// FIXME: make this more efficient
	results := make([]MessageEntry, 0)
	for rows.Next() {
		var match MessageEntry
		if err := rows.Scan(&match.Id, &match.Subject, &match.Body, &match.FromName, &match.FromAddress, &match.Date); err != nil {
			return nil, err
		}
		match.Body = strings.ReplaceAll(strings.TrimSpace(match.Body), "\n", " ")
		results = append(results, match)
	}

	return results, rows.Err()
}

type MailboxInfo struct {
	Id   int64
	Name string
}

func GetMailboxes(db *sql.DB) ([]MailboxInfo, error) {
	rows, err := db.Query("SELECT id, name FROM mailbox")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mailboxes := make([]MailboxInfo, 0)
	for rows.Next() {
		var row MailboxInfo
		if err := rows.Scan(&row.Id, &row.Name); err != nil {
			return nil, err
		}
		mailboxes = append(mailboxes, row)
	}
	return mailboxes, rows.Err()
}

type MessageEntry struct {
	Id          int64
	Subject     string
	FromName    string
	FromAddress string
	Date        time.Time
	Body        string
}

func GetMessages(db *sql.DB, mailboxID int64) ([]MessageEntry, error) {
	rows, err := db.Query("SELECT id, subject, from_name, from_address, date FROM message WHERE mailbox_id = ? ORDER BY date DESC", mailboxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []MessageEntry
	for rows.Next() {
		var message MessageEntry
		if err := rows.Scan(&message.Id, &message.Subject, &message.FromName, &message.FromAddress, &message.Date); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func GetMessageBody(db *sql.DB, messageId int64) (string, error) {
	var path string
	if err := db.QueryRow("SELECT path FROM message WHERE id = ?", messageId).Scan(&path); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(raw), nil
}
