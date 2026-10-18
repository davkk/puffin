package mail

import (
	_ "embed"
	"path/filepath"
	"puffin/pkg/assert"

	"database/sql"
	"fmt"
	"os"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	_ "modernc.org/sqlite"
)

type Mailbox struct {
	Name          string
	ID            uint64
	UIDNext       imap.UID
	UIDValidity   uint32
	HighestModSeq uint64
	NumMessages   uint64
}

func deleteFilesFromRows(rows *sql.Rows) error {
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %v", path, err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	// TODO: remove empty dirs
	return nil
}

func (mbox *Mailbox) reconcile(db *sql.DB, imapClient *imapclient.Client) error {
	assert.Assert(db != nil, "db is nil")
	assert.Assert(imapClient != nil, "imapClient is nil")
	assert.Assert(mbox.ID > 0, "mailbox ID is invalid")

	searchAllData, err := imapClient.UIDSearch(&imap.SearchCriteria{}, &imap.SearchOptions{ReturnAll: true}).Wait()
	if err != nil {
		return fmt.Errorf("search all data: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.Exec("create temp table if not exists _server_uids (uid integer primary key)"); err != nil {
		return err
	}

	if _, err := tx.Exec("delete from _server_uids"); err != nil {
		return err
	}

	stmt, err := tx.Prepare("insert or ignore into _server_uids(uid) values (?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, uid := range searchAllData.AllUIDs() {
		if _, err := stmt.Exec(uid); err != nil {
			return err
		}
	}

	rows, err := tx.Query(`
		select path from message
		where mailbox_id = ?
		and not exists (
			select 1 from _server_uids s where s.uid = message.uid
		)
	`, mbox.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	if err := deleteFilesFromRows(rows); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		delete from message
		where mailbox_id = ?
		and not exists (
			select 1 from _server_uids s where s.uid = message.uid
		)
	`, mbox.ID); err != nil {
		return err
	}

	if _, err := tx.Exec("drop table if exists _server_uids"); err != nil {
		return err
	}

	return tx.Commit()
}

func (mbox *Mailbox) updateChangedSince(db *sql.DB, imapClient *imapclient.Client) error {
	assert.Assert(db != nil, "database is nil")
	assert.Assert(mbox != nil, "mailbox is nil")
	assert.Assert(mbox.UIDNext > 1, "UIDNext is invalid")
	assert.Assert(mbox.HighestModSeq > 0, "HighestModSeq is invalid")

	fetchModOptions := &imap.FetchOptions{
		UID:          true,
		Flags:        true,
		ChangedSince: mbox.HighestModSeq,
	}
	var allUIDs imap.UIDSet
	allUIDs.AddRange(1, mbox.UIDNext-1)

	modMessages, err := imapClient.Fetch(allUIDs, fetchModOptions).Collect()
	if err != nil {
		return fmt.Errorf("fetch modified: %w", err)
	}
	for _, msg := range modMessages {
		// TODO: explore other changes to mail other than flags (moves, labels, etc.)
		// TODO: store flags in separate table
		flags := ParseFlags(msg.Flags)
		_, err := db.Exec("update message set flags = ?, modseq = ? where mailbox_id = ? and uid = ?",
			flags, msg.ModSeq, mbox.ID, msg.UID)
		if err != nil {
			return fmt.Errorf("update message: %w", err)
		}
	}
	return nil
}

func (mbox *Mailbox) updateLocalState(db *sql.DB, selectData *imap.SelectData) error {
	assert.Assert(db != nil, "database is nil")
	assert.Assert(mbox.ID > 0, "mailbox id is invalid")
	assert.Assert(selectData != nil, "mailbox is nil")

	_, err := db.Exec(`
		update mailbox
		set uid_next = ?, uid_validity = ?, highest_modseq = ?, last_sync = current_timestamp
		where id = ?
	`, selectData.UIDNext, selectData.UIDValidity, selectData.HighestModSeq, mbox.ID)
	if err != nil {
		return fmt.Errorf("update mailbox state: %w", err)
	}
	return nil
}

func (mbox *Mailbox) saveToFile(uid imap.UID, raw []byte) (string, error) {
	mboxDir := fmt.Sprintf("%s_%d", mbox.Name, mbox.ID)
	path := filepath.Join("userdata", mboxDir) // TODO: make sure this is unique
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}

	fileName := fmt.Sprintf("%d.eml", uid)
	path = filepath.Join(path, fileName)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		return "", err
	}

	return path, nil
}

func (mbox *Mailbox) fetchMessages(db *sql.DB, imapClient *imapclient.Client, rangeUIDs imap.UIDSet) error {
	assert.Assert(db != nil, "database is nil")
	assert.Assert(imapClient != nil, "imap client is nil")
	assert.Assert(mbox.ID > 0, "mailbox id is invalid")
	assert.Assert(len(rangeUIDs) > 0, "rangeUIDs is empty")

	bodySection := &imap.FetchItemBodySection{Peek: true}
	fetchOptions := &imap.FetchOptions{
		Envelope:    true, // with all the contents
		UID:         true,
		Flags:       true,
		BodySection: []*imap.FetchItemBodySection{bodySection},
	}

	messages, err := imapClient.Fetch(rangeUIDs, fetchOptions).Collect()
	if err != nil {
		return fmt.Errorf("fetch new: %w", err)
	}

	for _, msg := range messages {
		raw := msg.FindBodySection(bodySection)
		savedPath, err := mbox.saveToFile(msg.UID, raw)
		if err != nil {
			return fmt.Errorf("save mail: %w", err)
		}
		mail, err := NewMail(msg, raw)
		if err != nil {
			return fmt.Errorf("parse mail: %w", err)
		}
		_, err = db.Exec(`
			insert into message (mailbox_id, path, flags, uid, subject, from_addr, date, body)
			values (?, ?, ?, ?, ?, ?, ?, ?)
		`,
			mbox.ID, savedPath, mail.Flags, mail.UID, mail.Subject, mail.FromAddr, mail.Date, mail.Body)
		if err != nil {
			return fmt.Errorf("insert message: %w", err)
		}
	}

	return nil
}

func NewMailbox(db *sql.DB, mboxName string) (*Mailbox, error) {
	assert.Assert(db != nil, "db is nil")
	assert.Assert(mboxName != "", "mailbox name is empty")

	mbox := &Mailbox{Name: mboxName}

	// ensure mailbox exists in db
	_, err := db.Exec("insert or ignore into mailbox (name) values (?)", mbox.Name)
	if err != nil {
		return nil, fmt.Errorf("ensure mailbox: %w", err)
	}

	// fetch the mailbox from db
	err = db.QueryRow("select id, uid_validity, uid_next, highest_modseq from mailbox where name = ?", mbox.Name).
		Scan(&mbox.ID, &mbox.UIDValidity, &mbox.UIDNext, &mbox.HighestModSeq)
	if err != nil {
		return nil, fmt.Errorf("query mailbox: %w", err)
	}

	return mbox, nil
}

// TODO: wrap db calls in transaction
func (mbox *Mailbox) Sync(db *sql.DB, imapClient *imapclient.Client) error {
	assert.Assert(db != nil, "db is nil")
	assert.Assert(imapClient != nil, "imapClient is nil")
	assert.Assert(mbox.Name != "", "mailboxName is empty")

	// select mailbox in imap
	selectOptions := &imap.SelectOptions{CondStore: true}
	imapMbox, err := imapClient.Select(mbox.Name, selectOptions).Wait()
	if err != nil {
		return fmt.Errorf("select mailbox %s: %w", mbox.Name, err)
	}

	if mbox.UIDValidity > 0 && mbox.UIDValidity != imapMbox.UIDValidity {
		// TODO: prune existing mailbox & messages
		fmt.Fprintf(os.Stderr, "UID validity mismatch: %d != %d", mbox.UIDValidity, imapMbox.UIDValidity)
		mbox.UIDValidity = 0
		mbox.UIDNext = 1
	}

	if mbox.UIDNext < imapMbox.UIDNext {
		var newUIDs imap.UIDSet
		newUIDs.AddRange(mbox.UIDNext, imapMbox.UIDNext-1)
		if err := mbox.fetchMessages(db, imapClient, newUIDs); err != nil {
			return fmt.Errorf("fetch messages: %w", err)
		}
		mbox.UIDNext = imapMbox.UIDNext
	}

	if mbox.HighestModSeq > 0 && mbox.HighestModSeq < imapMbox.HighestModSeq {
		if err := mbox.reconcile(db, imapClient); err != nil {
			return fmt.Errorf("reconcile: %w", err)
		}
		if err := mbox.updateChangedSince(db, imapClient); err != nil {
			return fmt.Errorf("update changed since: %w", err)
		}
	}

	return mbox.updateLocalState(db, imapMbox)
}
