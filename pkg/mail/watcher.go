package mail

import (
	"context"
	"database/sql"
	"log"
	"puffin/pkg/assert"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"
)

// TODO: add watcher per account
type MailboxWatcher struct {
	mailboxName string
	imapClient  *imapclient.Client
	wakeUpCh    chan struct{}
}

func NewMailboxWatcher(mailbox string) *MailboxWatcher {
	return &MailboxWatcher{
		mailboxName: mailbox,
		wakeUpCh:    make(chan struct{}),
	}
}

func (mw *MailboxWatcher) WakeUp() {
	select {
	case mw.wakeUpCh <- struct{}{}:
	default:
	}
}

func (mw *MailboxWatcher) getDataHandler() *imapclient.UnilateralDataHandler {
	return &imapclient.UnilateralDataHandler{
		Mailbox: func(data *imapclient.UnilateralDataMailbox) {
			if data.NumMessages != nil {
				mw.WakeUp()
			}
		},
		Expunge: func(seqNum uint32) {
			mw.WakeUp()
		},
		Fetch: func(msg *imapclient.FetchMessageData) {
			mw.WakeUp()
		},
	}

}

// TODO: add WatchAll, with imapClient.List("", "*", nil)
func (mw *MailboxWatcher) Watch(ctx context.Context, db *sql.DB) {
	var err error
	// TODO: extract connection logic
	mw.imapClient, err = ConnectImapClient("localhost:143", "test", "password", mw.getDataHandler())
	assert.NoError(err, "failed to connect to imap server")
	defer mw.imapClient.Logout()

	for {
		// TODO: add reconnect logic if tcp dies
		if err := SyncMailbox(db, mw.imapClient, mw.mailboxName); err != nil {
			log.Printf("sync error: %v", err)
		}

		log.Println("enter idle state")
		idleCmd, err := mw.imapClient.Idle()
		if err != nil {
			log.Printf("IDLE error: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		select {
		case <-ctx.Done():
			idleCmd.Close()
			idleCmd.Wait()
			return
		case <-time.After(29 * time.Minute):
			log.Println("refresh idle")
			idleCmd.Close()
		case <-mw.wakeUpCh:
			log.Println("server event received")
			idleCmd.Close()
		}

		if err := idleCmd.Wait(); err != nil {
			log.Printf("IDLE wait error: %v", err)
		}
	}
}
