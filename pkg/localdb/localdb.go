package localdb

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"puffin/pkg/assert"
	"time"
)

//go:embed schema.sql
var sqlSchema string

type LocalDB struct {
	DB *sql.DB
}

func NewLocalDB(filepath string) (*LocalDB, error) {
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
	return &LocalDB{db}, nil
}

type MailboxInfo struct {
	Id   int64
	Name string
}

func (local *LocalDB) GetMailboxes() ([]MailboxInfo, error) {
	rows, err := local.DB.Query("select id, name from mailbox")
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
	Id       int64
	Subject  string
	FromAddr string
	Date     time.Time
	Body     string
}

func (local *LocalDB) GetMessages(mailboxID int64) ([]MessageEntry, error) {
	rows, err := local.DB.Query("select id, subject, from_addr, date from message where mailbox_id = ? order by date desc", mailboxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []MessageEntry
	for rows.Next() {
		var message MessageEntry
		if err := rows.Scan(&message.Id, &message.Subject, &message.FromAddr, &message.Date); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (local *LocalDB) GetMessageBody(messageId int64) (string, error) {
	var path string
	if err := local.DB.QueryRow("select path from message where id = ?", messageId).Scan(&path); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(raw), nil
}
