package main

import (
	"os"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func main() {
	app := gtk.NewApplication("com.puffin.app", gio.ApplicationFlagsNone)
	app.ConnectActivate(func() { activate(app) })

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}

func activate(app *gtk.Application) {
	win := gtk.NewApplicationWindow(app)
	win.SetDefaultSize(1280, 720)

	head := gtk.NewHeaderBar()
	head.SetShowTitleButtons(true)
	win.SetTitlebar(head)

	sidebar := gtk.NewListBox()
	sidebar.SetSizeRequest(200, -1)

	mailboxes := []string{"Inbox", "Sent", "Drafts", "Trash", "Archive"}
	for _, m := range mailboxes {
		row := gtk.NewListBoxRow()
		label := gtk.NewLabel(m)
		label.SetHAlign(gtk.AlignStart)
		label.SetMarginStart(8)
		label.SetMarginEnd(8)
		label.SetMarginTop(8)
		label.SetMarginBottom(8)
		box := gtk.NewBox(gtk.OrientationHorizontal, 0)
		box.Append(label)
		row.SetChild(box)
		sidebar.Append(row)
	}

	listStore := gtk.NewListStore([]glib.Type{glib.TypeString, glib.TypeString, glib.TypeString})
	for i := 1; i <= 20; i++ {
		iter := listStore.Append()
		listStore.SetValue(iter, 0, glib.NewValue("sender@example.com"))
		listStore.SetValue(iter, 1, glib.NewValue("Subject line "+string(rune('A'+i%26))))
		listStore.SetValue(iter, 2, glib.NewValue("May 10"))
	}

	treeView := gtk.NewTreeView()

	colFrom := gtk.NewTreeViewColumn()
	colFrom.SetTitle("From")
	colFrom.SetFixedWidth(300)
	rendererFrom := gtk.NewCellRendererText()
	colFrom.PackStart(rendererFrom, true)
	colFrom.AddAttribute(rendererFrom, "text", 0)
	treeView.AppendColumn(colFrom)

	colSubject := gtk.NewTreeViewColumn()
	colSubject.SetTitle("Subject")
	colSubject.SetFixedWidth(400)
	colSubject.SetResizable(true)
	rendererSubject := gtk.NewCellRendererText()
	colSubject.PackStart(rendererSubject, true)
	colSubject.AddAttribute(rendererSubject, "text", 1)
	treeView.AppendColumn(colSubject)

	colDate := gtk.NewTreeViewColumn()
	colDate.SetTitle("Date")
	colDate.SetFixedWidth(100)
	rendererDate := gtk.NewCellRendererText()
	colDate.PackStart(rendererDate, true)
	colDate.AddAttribute(rendererDate, "text", 2)
	treeView.AppendColumn(colDate)

	treeView.SetModel(listStore)

	scrolledMail := gtk.NewScrolledWindow()
	scrolledMail.SetPolicy(gtk.PolicyAutomatic, gtk.PolicyAutomatic)
	scrolledMail.SetChild(treeView)

	paned := gtk.NewPaned(gtk.OrientationHorizontal)
	paned.SetStartChild(sidebar)
	paned.SetEndChild(scrolledMail)
	paned.SetPosition(200)

	win.SetTitle("Puffin Mail")
	win.SetChild(paned)
	win.Show()
}
