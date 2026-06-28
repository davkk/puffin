package ui

import (
	"database/sql"
	"fmt"
	"html"
	"strconv"
	"strings"

	"puffin/pkg/assert"
	"puffin/pkg/localdb"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func escapeMarkup(s string) string {
	s = html.EscapeString(s)
	s = strings.ReplaceAll(s, "&lt;b&gt;", "<b>")
	s = strings.ReplaceAll(s, "&lt;/b&gt;", "</b>")
	return s
}

const SIDEBAR_WIDTH = 250

type MainWindow struct {
	*gtk.Window
	db     *sql.DB
	mailCh chan string

	searchEntry     *gtk.SearchEntry
	listBox         *gtk.ListBox
	contentBuffer   *gtk.TextBuffer
	sidebarList     *gtk.ListBox
	contentScrolled *gtk.ScrolledWindow

	outerPaned     *gtk.Paned
	msgPane        *gtk.Paned
	hamburger      *gtk.Button
	sidebarVisible bool

	selectedMailbox int64
	messageIDs      []string
	mailboxIDs      []int64
}

func NewMainWindow(app *gtk.Application, db *sql.DB, mailCh chan string) *MainWindow {
	win := gtk.NewWindow()
	win.SetDefaultSize(1400, 800)

	vbox := gtk.NewBox(gtk.OrientationVertical, 0)
	win.SetChild(vbox)

	mw := &MainWindow{Window: win, db: db, mailCh: mailCh, sidebarVisible: true}

	vbox.Append(mw.buildTopBar())
	vbox.Append(mw.buildBody())

	mailboxes, err := localdb.GetMailboxes(mw.db)
	assert.NoError(err, "failed to get mailboxes")

	mw.renderMailboxes(mailboxes)
	mw.setupGlobalShortcuts()
	mw.startMailListener()

	return mw
}

// ---------- Layout ----------

func (mw *MainWindow) buildTopBar() gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationHorizontal, 0)
	box.SetHExpand(true)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	hamburger := gtk.NewButtonFromIconName("sidebar-hide-symbolic")
	mw.hamburger = hamburger
	box.Append(hamburger)

	searchEntry := mw.buildSearchEntry()
	box.Append(searchEntry)

	hamburger.ConnectClicked(func() {
		mw.toggleSidebar()
	})

	return box
}

func (mw *MainWindow) buildSearchEntry() *gtk.SearchEntry {
	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetPlaceholderText("Search")
	searchEntry.SetWidthChars(60)
	mw.searchEntry = searchEntry

	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval uint, keycode uint, state gdk.ModifierType) bool {
		if keyval == gdk.KEY_Down {
			if first := mw.listBox.FirstChild(); first != nil {
				row := first.(*gtk.ListBoxRow)
				mw.listBox.SelectRow(row)
				row.GrabFocus()
			}
			return true
		}
		return false
	})
	searchEntry.AddController(keyCtrl)

	searchEntry.ConnectSearchChanged(func() {
		query := searchEntry.Text()
		mw.clearList()

		if query == "" {
			mw.showMailboxMessages()
			return
		}

		results, err := localdb.Search(mw.db, query)
		if err != nil {
			mw.showListMessage("Search error\n" + err.Error())
			return
		}
		if len(results) == 0 {
			mw.showListMessage("No results found.")
			return
		}

		mw.messageIDs = make([]string, 0, len(results))
		for _, r := range results {
			mw.appendRow(r)
			mw.messageIDs = append(mw.messageIDs, strconv.FormatInt(r.Id, 10))
		}
	})

	searchEntry.ConnectStopSearch(func() {
		searchEntry.SetText("")
		mw.showMailboxMessages()
	})

	return searchEntry
}

func (mw *MainWindow) buildBody() gtk.Widgetter {
	outerPaned := gtk.NewPaned(gtk.OrientationHorizontal)
	outerPaned.SetHExpand(true)
	outerPaned.SetVExpand(true)
	mw.outerPaned = outerPaned

	outerPaned.SetStartChild(mw.buildSidebar())
	outerPaned.SetEndChild(mw.buildMessagePane())

	outerPaned.SetPosition(SIDEBAR_WIDTH)

	return outerPaned
}

func (mw *MainWindow) toggleSidebar() {
	mw.sidebarVisible = !mw.sidebarVisible
	if mw.sidebarVisible {
		mw.outerPaned.SetPosition(SIDEBAR_WIDTH)
		mw.hamburger.SetIconName("sidebar-hide-symbolic")
	} else {
		mw.outerPaned.SetPosition(0)
		mw.hamburger.SetIconName("sidebar-show-symbolic")
	}
}

func (mw *MainWindow) buildSidebar() gtk.Widgetter {
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolled.SetSizeRequest(160, -1)

	scrolled.AddCSSClass("sidebar")

	css := gtk.NewCSSProvider()
	css.LoadFromString(".sidebar { background: shade(@theme_bg_color, 0.92); }")
	gtk.StyleContextAddProviderForDisplay(scrolled.Display(), css, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)

	sidebarList := gtk.NewListBox()
	mw.sidebarList = sidebarList
	scrolled.SetChild(sidebarList)

	sidebarList.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		index := row.Index()
		if index < 0 || index >= len(mw.mailboxIDs) {
			return
		}
		mw.selectedMailbox = mw.mailboxIDs[index]
		mw.searchEntry.SetText("")
		mw.showMailboxMessages()
	})

	return scrolled
}

func (mw *MainWindow) buildMessagePane() gtk.Widgetter {
	msgPane := gtk.NewPaned(gtk.OrientationHorizontal)
	msgPane.SetHExpand(true)
	msgPane.SetVExpand(true)
	msgPane.SetResizeStartChild(true)
	msgPane.SetResizeEndChild(true)
	mw.msgPane = msgPane

	msgPane.SetStartChild(mw.buildMessageList())
	msgPane.SetEndChild(mw.buildContentView())

	return msgPane
}

func (mw *MainWindow) buildMessageList() gtk.Widgetter {
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolled.SetVExpand(true)

	listBox := gtk.NewListBox()
	mw.listBox = listBox
	scrolled.SetChild(listBox)

	listBox.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		mw.showMessageContent(row)
	})

	listBox.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row == nil && mw.searchEntry.Text() == "" {
			mw.contentScrolled.SetVisible(false)
		}
	})

	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.SetPropagationPhase(gtk.PhaseCapture)
	keyCtrl.ConnectKeyPressed(func(keyval uint, keycode uint, state gdk.ModifierType) bool {
		sel := listBox.SelectedRow()
		switch keyval {
		case gdk.KEY_Up:
			if sel == nil || sel.Index() == 0 {
				mw.searchEntry.GrabFocus()
				listBox.UnselectAll()
				return true
			}
		case gdk.KEY_Escape:
			if sel != nil {
				listBox.UnselectAll()
				return true
			}
			mw.searchEntry.GrabFocus()
			return true
		}
		return false
	})
	listBox.AddController(keyCtrl)

	return scrolled
}

func (mw *MainWindow) buildContentView() gtk.Widgetter {
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyAutomatic, gtk.PolicyAutomatic)
	scrolled.SetVExpand(true)
	scrolled.SetVisible(false)
	mw.contentScrolled = scrolled

	textView := gtk.NewTextView()
	textView.SetEditable(false)
	textView.SetCursorVisible(false)
	textView.SetWrapMode(gtk.WrapWord)
	textView.AddCSSClass("puffin-mono")
	mw.contentBuffer = textView.Buffer()

	css := gtk.NewCSSProvider()
	css.LoadFromString(".puffin-mono { font-family: monospace; }")
	gtk.StyleContextAddProviderForDisplay(textView.Display(), css, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)

	scrolled.SetChild(textView)
	return scrolled
}

// ---------- Global shortcuts ----------

func (mw *MainWindow) setupGlobalShortcuts() {
	ctrl := gtk.NewEventControllerKey()
	ctrl.SetPropagationPhase(gtk.PhaseCapture)
	ctrl.ConnectKeyPressed(func(keyval uint, keycode uint, state gdk.ModifierType) bool {
		switch {
		case keyval == gdk.KEY_e && state&gdk.ControlMask != 0:
			mw.toggleSidebar()
			return true
		case keyval == gdk.KEY_f && state&gdk.ControlMask != 0:
			mw.focusSearch()
			return true
		case keyval == gdk.KEY_slash:
			mw.focusSearch()
			return true
		case keyval == gdk.KEY_Escape:
			if sel := mw.listBox.SelectedRow(); sel != nil {
				mw.listBox.UnselectAll()
				return true
			}
		}
		return false
	})
	mw.AddController(ctrl)
}

func (mw *MainWindow) focusSearch() {
	mw.searchEntry.GrabFocus()
	mw.searchEntry.SelectRegion(0, -1)
}

// ---------- Data loading ----------

func (mw *MainWindow) renderMailboxes(mailboxes []localdb.MailboxInfo) {
	mw.mailboxIDs = make([]int64, 0, len(mailboxes))
	for _, mb := range mailboxes {
		label := gtk.NewLabel(mb.Name)
		label.SetXAlign(0)
		label.SetMarginStart(12)
		label.SetMarginEnd(12)
		label.SetMarginTop(6)
		label.SetMarginBottom(6)

		row := gtk.NewListBoxRow()
		row.SetChild(label)
		row.SetFocusable(true)
		mw.sidebarList.Append(row)

		mw.mailboxIDs = append(mw.mailboxIDs, mb.Id)
	}
}

func (mw *MainWindow) showMailboxMessages() {
	mw.clearList()

	if mw.selectedMailbox == 0 {
		mw.showListMessage("Select a mailbox")
		return
	}

	messages, err := localdb.GetMessages(mw.db, mw.selectedMailbox) // FIXME: make the function more testable, more dumb
	if err != nil {
		mw.showListMessage("Error loading messages\n" + err.Error())
		return
	}
	if len(messages) == 0 {
		mw.showListMessage("No messages in this mailbox.")
		return
	}

	mw.messageIDs = make([]string, 0, len(messages))
	for _, msg := range messages {
		mw.appendRow(msg)
		mw.messageIDs = append(mw.messageIDs, strconv.FormatInt(msg.Id, 10))
	}
}

func (mw *MainWindow) showMessageContent(row *gtk.ListBoxRow) {
	index := row.Index()
	if index < 0 || index >= len(mw.messageIDs) {
		return
	}

	id, err := strconv.ParseInt(mw.messageIDs[index], 10, 64)
	if err != nil {
		return
	}

	body, err := localdb.GetMessageBody(mw.db, id)
	if err != nil {
		mw.contentBuffer.SetText("Error loading content: " + err.Error())
	} else {
		mw.contentBuffer.SetText(body)
	}

	mw.revealContentPane()
}

func (mw *MainWindow) revealContentPane() {
	mw.contentScrolled.SetVisible(true)

	innerWidth := mw.Window.Width() - mw.outerPaned.Position()
	if innerWidth > 0 {
		mw.msgPane.SetPosition(innerWidth / 2)
	}
}

func (mw *MainWindow) startMailListener() {
	go func() {
		for range mw.mailCh {
			glib.IdleAdd(func() bool {
				if mw.searchEntry.Text() == "" {
					mw.showMailboxMessages()
				}
				return false
			})
		}
	}()
}

// ---------- Row helpers ----------

func (mw *MainWindow) clearList() {
	mw.messageIDs = nil
	for row := mw.listBox.FirstChild(); row != nil; row = mw.listBox.FirstChild() {
		mw.listBox.Remove(row)
	}
}

func (mw *MainWindow) showListMessage(markup string) {
	label := gtk.NewLabel("")
	label.SetUseMarkup(true)
	label.SetMarkup(markup)

	row := gtk.NewListBoxRow()
	row.SetChild(label)
	row.SetFocusable(true)
	mw.listBox.Append(row)
}

func (mw *MainWindow) appendRow(msg localdb.MessageEntry) {
	rowBox := gtk.NewBox(gtk.OrientationVertical, 2)
	rowBox.SetMarginStart(6)
	rowBox.SetMarginEnd(6)
	rowBox.SetMarginTop(4)
	rowBox.SetMarginBottom(4)

	subjectLabel := gtk.NewLabel("")
	subjectLabel.SetUseMarkup(true)
	subjectLabel.SetXAlign(0)
	subjectLabel.SetWrap(true)
	subjectLabel.SetMarkup(escapeMarkup(msg.Subject))
	rowBox.Append(subjectLabel)

	if msg.FromName != "" {
		fromLabel := gtk.NewLabel("")
		fromLabel.SetXAlign(0)
		fromLabel.SetOpacity(0.7)
		fromLabel.SetText(fmt.Sprintf("%s \u2014 %s", msg.FromName, msg.Date.Format("Jan 2, 2006")))
		rowBox.Append(fromLabel)
	}

	if msg.Body != "" {
		bodyLabel := gtk.NewLabel("")
		bodyLabel.SetUseMarkup(true)
		bodyLabel.SetXAlign(0)
		bodyLabel.SetWrap(true)
		bodyLabel.SetMaxWidthChars(80)
		bodyLabel.SetMarkup(escapeMarkup(msg.Body))
		rowBox.Append(bodyLabel)
	}

	row := gtk.NewListBoxRow()
	row.SetChild(rowBox)
	row.SetFocusable(true)
	mw.listBox.Append(row)
}
