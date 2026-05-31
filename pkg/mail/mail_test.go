package mail

import (
	"database/sql"
	"fmt"
	"os/exec"
	"puffin/internal/testutil"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"
)

var (
	plainEML     = sync.OnceValue(loadPlain)
	htmlEML      = sync.OnceValue(loadHTML)
	multipartEML = sync.OnceValue(loadMultipart)
	additionEML  = sync.OnceValue(loadAddition)
)

func loadPlain() []byte {
	b, err := testutil.ReadTestdata("plain.eml")
	if err != nil {
		panic(err)
	}
	return b
}

func loadHTML() []byte {
	b, err := testutil.ReadTestdata("html.eml")
	if err != nil {
		panic(err)
	}
	return b
}

func loadMultipart() []byte {
	b, err := testutil.ReadTestdata("multipart.eml")
	if err != nil {
		panic(err)
	}
	return b
}

func loadAddition() []byte {
	b, err := testutil.ReadTestdata("addition.eml")
	if err != nil {
		panic(err)
	}
	return b
}

type fixture struct {
	t       *testing.T
	db      *sql.DB
	client  *imapclient.Client
	mailbox string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	db, err := ConnectSqlite(":memory:")
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	client, err := ConnectImapClient("localhost:143", "test", "password", nil)
	if err != nil {
		t.Fatalf("connect imap: %v", err)
	}
	t.Cleanup(func() { client.Logout() })

	name := fmt.Sprintf("test-%d", time.Now().UnixNano())

	f := &fixture{t: t, db: db, client: client, mailbox: name}
	f.doveadm("", "mailbox", "create", "-u", "test", name)
	t.Cleanup(func() { f.doveadm("", "mailbox", "delete", "-u", "test", name) })
	return f
}

func (f *fixture) doveadm(stdin string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("docker", append([]string{"exec", "-i", "dovecot-test", "doveadm"}, args...)...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("doveadm %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (f *fixture) seed() {
	f.doveadm(string(plainEML()), "save", "-u", "test", "-m", f.mailbox)
	f.doveadm(string(htmlEML()), "save", "-u", "test", "-m", f.mailbox)
	f.doveadm(string(multipartEML()), "save", "-u", "test", "-m", f.mailbox)
}

func (f *fixture) dbUIDs() []int {
	rows, err := f.db.Query("SELECT uid FROM message ORDER BY uid")
	if err != nil {
		f.t.Fatalf("query uids: %v", err)
	}
	defer rows.Close()
	var uids []int
	for rows.Next() {
		var uid int
		if err := rows.Scan(&uid); err != nil {
			f.t.Fatalf("scan uid: %v", err)
		}
		uids = append(uids, uid)
	}
	return uids
}

func (f *fixture) dbCount() int {
	var n int
	f.db.QueryRow("SELECT COUNT(*) FROM message").Scan(&n)
	return n
}

func (f *fixture) dbFlags(uid int) string {
	var flags sql.NullString
	f.db.QueryRow("SELECT flags FROM message WHERE uid = ?", uid).Scan(&flags)
	return flags.String
}

func TestSync(t *testing.T) {
	f := newFixture(t)

	t.Run("initial sync seeds database", func(t *testing.T) {
		f.seed()

		if err := SyncMailbox(f.db, f.client, f.mailbox); err != nil {
			t.Fatalf("syncMail: %v", err)
		}

		if n := f.dbCount(); n != 3 {
			t.Errorf("expected 3 messages, got %d", n)
		}
	})

	t.Run("addition syncs new messages", func(t *testing.T) {
		f.doveadm(string(additionEML()), "save", "-u", "test", "-m", f.mailbox)

		if err := SyncMailbox(f.db, f.client, f.mailbox); err != nil {
			t.Fatalf("syncMail: %v", err)
		}

		if n := f.dbCount(); n != 4 {
			t.Errorf("expected 4 messages, got %d", n)
		}
	})

	t.Run("deletion removes message from database", func(t *testing.T) {
		uids := f.dbUIDs()
		if len(uids) == 0 {
			t.Fatal("no messages in db")
		}
		target := uids[0]

		f.doveadm("", "expunge", "-u", "test", "mailbox", f.mailbox, "uid", fmt.Sprintf("%d", target))

		if err := SyncMailbox(f.db, f.client, f.mailbox); err != nil {
			t.Fatalf("syncMail: %v", err)
		}

		for _, uid := range f.dbUIDs() {
			if uid == target {
				t.Errorf("message UID %d should have been deleted from db", target)
			}
		}
	})

	t.Run("flag changes update flags in database", func(t *testing.T) {
		uids := f.dbUIDs()
		if len(uids) == 0 {
			t.Fatal("no messages in db")
		}
		target := uids[len(uids)-1]

		f.doveadm("", "flags", "add", "-u", "test", `\Seen`, "mailbox", f.mailbox, "uid", fmt.Sprintf("%d", target))
		f.doveadm("", "flags", "add", "-u", "test", `\Flagged`, "mailbox", f.mailbox, "uid", fmt.Sprintf("%d", target))

		if err := SyncMailbox(f.db, f.client, f.mailbox); err != nil {
			t.Fatalf("syncMail: %v", err)
		}

		flags := f.dbFlags(target)
		if !strings.Contains(flags, "\\Seen") {
			t.Errorf("expected \\Seen in flags, got %q", flags)
		}
	})
}
