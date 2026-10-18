package mail

import (
	_ "embed"
	"puffin/pkg/assert"

	"bytes"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message"
	"golang.org/x/net/html"

	_ "modernc.org/sqlite"
)

// TODO: consider sqlc

type Mail struct {
	UID      imap.UID
	Subject  string
	FromAddr string
	Date     time.Time
	Body     string
	Flags    string
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

func stripHTML(input string) string {
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		return ""
	}
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			sb.WriteString(strings.TrimSpace(n.Data) + " ")
		case html.ElementNode:
			if n.Data == "script" || n.Data == "style" {
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return sb.String()
}

func NewMail(msg *imapclient.FetchMessageBuffer, raw []byte) (*Mail, error) {
	assert.Assert(msg.Envelope != nil, "envelope is nil")
	assert.Assert(len(msg.Envelope.From) > 0, "from in envelope is empty")

	email := &Mail{
		UID:      msg.UID,
		Subject:  msg.Envelope.Subject,
		FromAddr: msg.Envelope.From[0].Addr(),
		Date:     msg.Envelope.Date,
		Flags:    ParseFlags(msg.Flags),
	}

	entity, err := message.Read(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}

	walk := func(path []int, entity *message.Entity, err error) error {
		if err != nil {
			return err
		}
		contentType, _, err := entity.Header.ContentType()
		if err != nil {
			return err
		}

		isPlain := strings.Contains(contentType, "text/plain")
		isHtml := strings.Contains(contentType, "text/html")
		if isPlain || isHtml {
			buf := new(bytes.Buffer)
			buf.ReadFrom(entity.Body)
			email.Body += stripHTML(buf.String())
		}
		return nil
	}

	if err = entity.Walk(walk); err != nil {
		return nil, err
	}

	return email, nil
}
