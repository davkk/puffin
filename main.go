package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"puffin/pkg/assert"
	"puffin/pkg/mail"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"
	_ "modernc.org/sqlite"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	wakeUpCh := make(chan struct{}, 1)
	wakeUp := func() {
		select {
		case wakeUpCh <- struct{}{}:
		default:
		}
	}
	dataHandler := &imapclient.UnilateralDataHandler{
		Mailbox: func(data *imapclient.UnilateralDataMailbox) {
			if data.NumMessages != nil {
				wakeUp()
			}
		},
		Expunge: func(seqNum uint32) {
			wakeUp()
		},
		Fetch: func(msg *imapclient.FetchMessageData) {
			wakeUp()
		},
	}

	imapClient, err := mail.ConnectImapClient("localhost:143", "test", "password", dataHandler)
	assert.NoError(err, "failed to connect to imap server")
	defer imapClient.Logout()

	db, err := mail.ConnectSqlite("puffin.db")
	assert.NoError(err, "failed to connect to sqlite db")
	defer db.Close()

	for {
		// TODO: add reconnect logic if tcp dies
		if err := mail.SyncMailbox(db, imapClient, "INBOX"); err != nil {
			log.Printf("sync error: %v", err)
		}

		log.Println("enter idle state")
		idleCmd, err := imapClient.Idle()
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
		case <-wakeUpCh:
			log.Println("server event received")
			idleCmd.Close()
		}

		if err := idleCmd.Wait(); err != nil {
			log.Printf("IDLE wait error: %v", err)
		}
	}
}
