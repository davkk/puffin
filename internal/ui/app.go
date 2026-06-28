package ui

import (
	"context"
	"database/sql"
	"log"

	"puffin/pkg/localdb"
	"puffin/pkg/mail"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	_ "modernc.org/sqlite"
)

type App struct {
	app    *gtk.Application
	db     *sql.DB
	mailCh chan string
}

func NewApp() *App {
	a := &App{
		mailCh: make(chan string, 128),
	}

	app := gtk.NewApplication("com.puffin.email", 0)
	app.ConnectActivate(func() { a.activate(app) })

	a.app = app
	return a
}

func (a *App) Run(ctx context.Context) int {
	go a.mailWatcher(ctx)
	return a.app.Run([]string{})
}

func (a *App) activate(app *gtk.Application) {
	var err error
	a.db, err = localdb.ConnectSqlite("puffin.db")
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	win := NewMainWindow(app, a.db, a.mailCh)
	win.SetTitle("Puffin")
	win.SetDefaultSize(1200, 800)
	app.AddWindow(win.Window)
	win.SetVisible(true)
}

func (a *App) mailWatcher(ctx context.Context) {
	db, err := localdb.ConnectSqlite("puffin.db")
	if err != nil {
		log.Fatalf("watcher db: %v", err)
	}
	defer db.Close()

	mail.NewMailboxWatcher("INBOX").Watch(ctx, db, a.mailCh) // TODO: watch more than just inbox
}
