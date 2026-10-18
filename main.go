package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"puffin/pkg/localdb"
	"puffin/pkg/mail"

	_ "modernc.org/sqlite"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	local, err := localdb.NewLocalDB(envOr("PUFFIN_DB", "puffin.db"))
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer local.DB.Close()

	mailbox := envOr("PUFFIN_MAILBOX", "INBOX")

	// TODO: watch more than just one mailbox
	watcher := mail.NewMailboxWatcher(mailbox)
	log.Printf("watching %s (Ctrl+C to stop)", mailbox)
	watcher.Watch(ctx, local.DB, nil)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
