package mail

import (
	"puffin/pkg/assert"

	"crypto/tls"
	"fmt"
	"mime"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
)

func ConnectImapClient(serverAddr string, user string, pass string, dataHandler *imapclient.UnilateralDataHandler) (*imapclient.Client, error) {
	assert.Assert(serverAddr != "", "serverAddr is empty")
	assert.Assert(user != "", "user is empty")
	assert.Assert(pass != "", "pass is empty")

	opts := &imapclient.Options{
		WordDecoder:           &mime.WordDecoder{CharsetReader: charset.Reader},
		TLSConfig:             &tls.Config{InsecureSkipVerify: true},
		UnilateralDataHandler: dataHandler,
	}
	client, err := imapclient.DialStartTLS(serverAddr, opts)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	if err = client.Login(user, pass).Wait(); err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	if !CheckCapabilities(client) {
		client.Logout()
		return nil, fmt.Errorf("missing required IMAP capabilities")
	}
	return client, nil
}

func CheckCapabilities(imapClient *imapclient.Client) bool {
	assert.Assert(imapClient != nil, "imap client is nil")
	caps := imapClient.Caps()
	return caps.Has(imap.CapCondStore) && caps.Has(imap.CapIdle)
}
