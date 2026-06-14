package main

import (
	"context"
	"os"
	"os/signal"
	"puffin/pkg/assert"
	"puffin/pkg/mail"

	_ "modernc.org/sqlite"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	db, err := mail.ConnectSqlite("puffin.db")
	assert.NoError(err, "failed to connect to sqlite db")
	defer db.Close()

	go mail.NewMailboxWatcher("INBOX").Watch(ctx, db)
	<-ctx.Done()
}
