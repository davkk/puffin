package main

import (
	"puffin/pkg/assert"
	"puffin/pkg/mail"

	_ "modernc.org/sqlite"
)

func main() {
	imapClient, err := mail.ConnectImapClient("localhost:143", "test", "password")
	assert.NoError(err, "failed to connect to imap server")
	defer imapClient.Logout()

	db, err := mail.ConnectSqlite("puffin.db")
	assert.NoError(err, "failed to connect to sqlite db")
	defer db.Close()

	err = mail.SyncMailbox(db, imapClient, "INBOX")
	assert.NoError(err, "sync")

	// TODO: wire up IDLE with UnilateralDataHandler.Mailbox
	// ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	// defer cancel()
	// for {
	// 	select {
	// 	case <-ctx.Done():
	// 		fmt.Println("\nshutting down")
	// 		return
	// 	default:
	// 	}
	// 	err := syncMail(db, imapClient, "INBOX")
	// 	if err != nil {
	// 		fmt.Fprintf(os.Stderr, "sync error: %v\n", err)
	// 	}
	// 	idleCmd, err := imapClient.Idle()
	// 	if err != nil {
	// 		fmt.Fprintf(os.Stderr, "idle error: %v\n", err)
	// 		time.Sleep(5 * time.Second)
	// 		continue
	// 	}
	// 	select {
	// 	case <-ctx.Done():
	// 		idleCmd.Close()
	// 		fmt.Println("\nshutting down")
	// 		return
	// 	case <-time.After(30 * time.Second):
	// 		idleCmd.Close()
	// 	}
	// }
}
