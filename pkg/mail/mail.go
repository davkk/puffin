package mail

import (
	_ "embed"
	"puffin/pkg/assert"

	"bytes"
	"crypto/tls"
	"database/sql"
	"fmt"
	"mime"
	"os"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/charset"

	_ "modernc.org/sqlite"
)

// TODO: consider sqlc

//go:embed schema.sql
var sqlSchema string

type Mailbox struct {
	Name          string
	ID            uint64
	UIDNext       imap.UID
	UIDValidity   uint32
	HighestModSeq uint64
	NumMessages   uint64
}

type Mail struct {
	UID         imap.UID
	Subject     string
	FromName    string
	FromAddress string
	Date        time.Time
	Body        string
	Flags       string
}

func isRecent(flag imap.Flag) bool {
	return strings.EqualFold(string(flag), `\Recent`)
}

func isDeleted(flag imap.Flag) bool {
	return strings.EqualFold(string(flag), `\Deleted`)
}

func ParseFlags(flags []imap.Flag) string {
	var b strings.Builder
	first := true
	for _, f := range flags {
		if isRecent(f) {
			continue
		}
		if !first {
			b.WriteString(" ")
		}
		b.WriteString(string(f))
		first = false
	}
	return b.String()
}

func NewMail(msg *imapclient.FetchMessageBuffer, bodySection *imap.FetchItemBodySection) (*Mail, error) {
	assert.Assert(msg.Envelope != nil, "envelope is nil")
	assert.Assert(len(msg.Envelope.From) > 0, "from in envelope is empty")

	email := &Mail{
		UID:         msg.UID,
		Subject:     msg.Envelope.Subject,
		FromName:    msg.Envelope.From[0].Name,
		FromAddress: msg.Envelope.From[0].Addr(),
		Date:        msg.Envelope.Date,
		Flags:       ParseFlags(msg.Flags),
	}

	if section := msg.FindBodySection(bodySection); section != nil {
		entity, err := message.Read(bytes.NewReader(section))
		if err != nil {
			return nil, err
		}

		walker := func(path []int, entity *message.Entity, err error) error {
			if err != nil {
				return err
			}
			contentType, _, err := entity.Header.ContentType()
			if err != nil {
				return err
			}
			if strings.Contains(contentType, "text/plain") || strings.Contains(contentType, "text/html") {
				buf := new(bytes.Buffer)
				buf.ReadFrom(entity.Body)
				email.Body += buf.String()
			}
			return nil
		}

		if err = entity.Walk(walker); err != nil {
			return nil, err
		}
	}

	return email, nil
}

func ConnectImapClient(serverAddr string, user string, pass string) (*imapclient.Client, error) {
	assert.Assert(serverAddr != "", "serverAddr is empty")
	assert.Assert(user != "", "user is empty")
	assert.Assert(pass != "", "pass is empty")

	opts := &imapclient.Options{
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
		TLSConfig:   &tls.Config{InsecureSkipVerify: true},
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Expunge: func(seqNum uint32) {
				// TODO: expunge with deleted emails
				// TODO: i need seqNum -> UID mapping to delete the right ones, rebuild on every imap select?
			},
			Fetch: func(msg *imapclient.FetchMessageData) {
				// TODO: fetch real-time during IDLE for flag/label changes
			},
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				// TODO: mailbox status update (num messages, flags, etc.)
			},
		},
	}
	client, err := imapclient.DialStartTLS(serverAddr, opts)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	if err = client.Login(user, pass).Wait(); err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	if !CheckCapabilities(client) {
		client.Logout()
		return nil, fmt.Errorf("missing required IMAP capabilities")
	}
	return client, nil
}

func ConnectSqlite(filepath string) (*sql.DB, error) {
	assert.Assert(filepath != "", "empty filepath")

	db, err := sql.Open("sqlite", filepath)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if _, err = db.Exec(sqlSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	return db, nil
}

func reconcileMailbox(db *sql.DB, imapClient *imapclient.Client, mailboxID uint64) error {
	assert.Assert(db != nil, "db is nil")
	assert.Assert(imapClient != nil, "imapClient is nil")
	assert.Assert(mailboxID > 0, "mailboxID is invalid")

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

	_, err = tx.Exec("CREATE TEMP TABLE IF NOT EXISTS _server_uids (uid INTEGER PRIMARY KEY)")
	if err != nil {
		return err
	}

	_, err = tx.Exec("DELETE FROM _server_uids")
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT OR IGNORE INTO _server_uids(uid) VALUES (?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, uid := range searchAllData.AllUIDs() {
		if _, err := stmt.Exec(uid); err != nil {
			return err
		}
	}

	_, err = tx.Exec(`
		DELETE FROM message
		WHERE mailbox_id = ?
		AND NOT EXISTS (
			SELECT 1
			FROM _server_uids s
			WHERE s.uid = message.uid
		)
	`, mailboxID)
	if err != nil {
		return err
	}

	_, err = tx.Exec("DROP TABLE IF EXISTS _server_uids")
	if err != nil {
		return err
	}

	return tx.Commit()
}

func updateChangedSince(db *sql.DB, imapClient *imapclient.Client, mailbox *Mailbox) error {
	assert.Assert(db != nil, "database is nil")
	assert.Assert(mailbox != nil, "mailbox is nil")
	assert.Assert(mailbox.UIDNext > 1, "UIDNext is invalid")
	assert.Assert(mailbox.HighestModSeq > 0, "HighestModSeq is invalid")

	fetchModOptions := &imap.FetchOptions{
		UID:          true,
		Flags:        true,
		ChangedSince: mailbox.HighestModSeq,
	}
	var allUIDs imap.UIDSet
	allUIDs.AddRange(1, mailbox.UIDNext-1)

	modMessages, err := imapClient.Fetch(allUIDs, fetchModOptions).Collect()
	if err != nil {
		return fmt.Errorf("fetch modified: %w", err)
	}
	for _, msg := range modMessages {
		// TODO: explore other changes to mail other than flags (moves, labels, etc.)
		// TODO: store flags in separate table
		_, err := db.Exec("UPDATE message SET flags = ?, modseq = ? WHERE mailbox_id = ? AND uid = ?",
			ParseFlags(msg.Flags), msg.ModSeq, mailbox.ID, msg.UID)
		if err != nil {
			return fmt.Errorf("update message: %w", err)
		}
	}
	return nil
}

func updateMailboxState(db *sql.DB, mailboxID uint64, selectData *imap.SelectData) error {
	assert.Assert(db != nil, "database is nil")
	assert.Assert(mailboxID > 0, "mailbox id is invalid")
	assert.Assert(selectData != nil, "mailbox is nil")

	_, err := db.Exec(`
		UPDATE mailbox
		SET uid_next = ?, uid_validity = ?, highest_modseq = ?, last_sync = CURRENT_TIMESTAMP
		WHERE id = ?
	`, selectData.UIDNext, selectData.UIDValidity, selectData.HighestModSeq, mailboxID)
	if err != nil {
		return fmt.Errorf("update mailbox state: %w", err)
	}
	return nil
}

func fetchMessages(db *sql.DB, imapClient *imapclient.Client, mailboxID uint64, rangeUIDs imap.UIDSet) error {
	assert.Assert(db != nil, "database is nil")
	assert.Assert(imapClient != nil, "imap client is nil")
	assert.Assert(mailboxID > 0, "mailbox id is invalid")
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
		mail, err := NewMail(msg, bodySection)
		if err != nil {
			return fmt.Errorf("parse mail: %w", err)
		}
		_, err = db.Exec("INSERT INTO message (mailbox_id, flags, uid, subject, from_name, from_address, date, body) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			mailboxID, mail.Flags, mail.UID, mail.Subject, mail.FromName, mail.FromAddress, mail.Date, mail.Body)
		if err != nil {
			return fmt.Errorf("insert message: %w", err)
		}
	}

	return nil
}

func openMailbox(db *sql.DB, mailboxName string) (*Mailbox, error) {
	assert.Assert(db != nil, "db is nil")
	assert.Assert(mailboxName != "", "mailbox name is empty")

	mbox := &Mailbox{Name: mailboxName}

	// ensure mailbox exists in db
	_, err := db.Exec("INSERT OR IGNORE INTO mailbox (name) VALUES (?)", mbox.Name)
	if err != nil {
		return nil, fmt.Errorf("ensure mailbox: %w", err)
	}

	// fetch the mailbox from db
	err = db.QueryRow("SELECT id, uid_validity, uid_next, highest_modseq FROM mailbox WHERE name = ?", mbox.Name).
		Scan(&mbox.ID, &mbox.UIDValidity, &mbox.UIDNext, &mbox.HighestModSeq)
	if err != nil {
		return nil, fmt.Errorf("query mailbox: %w", err)
	}

	return mbox, nil
}

// TODO: wrap db calls in transaction
func SyncMailbox(db *sql.DB, imapClient *imapclient.Client, mailboxName string) error {
	assert.Assert(db != nil, "db is nil")
	assert.Assert(imapClient != nil, "imapClient is nil")
	assert.Assert(mailboxName != "", "mailboxName is empty")

	localMbox, err := openMailbox(db, mailboxName)
	if err != nil {
		return fmt.Errorf("open mailbox %s: %w", mailboxName, err)
	}

	// select mailbox in imap
	selectOptions := &imap.SelectOptions{CondStore: true}
	imapMbox, err := imapClient.Select(localMbox.Name, selectOptions).Wait()
	if err != nil {
		return fmt.Errorf("select mailbox %s: %w", localMbox.Name, err)
	}

	err = db.QueryRow("SELECT COUNT(*) FROM message WHERE mailbox_id = ?", localMbox.ID).Scan(&localMbox.NumMessages)
	if err != nil {
		return fmt.Errorf("message count failed for mailbox %s: %w", localMbox.Name, err)
	}

	if localMbox.UIDValidity > 0 && localMbox.UIDValidity != imapMbox.UIDValidity {
		// TODO: prune existing mailbox & messages
		fmt.Fprintf(os.Stderr, "UID validity mismatch: %d != %d", localMbox.UIDValidity, imapMbox.UIDValidity)
		localMbox.UIDValidity = 0
		localMbox.UIDNext = 1
	}

	if imapMbox.UIDNext > localMbox.UIDNext {
		var newUIDs imap.UIDSet
		newUIDs.AddRange(localMbox.UIDNext, imapMbox.UIDNext-1)
		if err := fetchMessages(db, imapClient, localMbox.ID, newUIDs); err != nil {
			return fmt.Errorf("fetch messages: %w", err)
		}
	}

	if localMbox.HighestModSeq > 0 && localMbox.HighestModSeq < imapMbox.HighestModSeq {
		if err := reconcileMailbox(db, imapClient, localMbox.ID); err != nil {
			return fmt.Errorf("reconcile: %w", err)
		}
		if err := updateChangedSince(db, imapClient, localMbox); err != nil {
			return fmt.Errorf("update changed since: %w", err)
		}
	}

	return updateMailboxState(db, localMbox.ID, imapMbox)
}

func CheckCapabilities(imapClient *imapclient.Client) bool {
	assert.Assert(imapClient != nil, "imap client is nil")
	caps := imapClient.Caps()
	return caps.Has(imap.CapCondStore) && caps.Has(imap.CapIdle)
}
