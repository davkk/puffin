package mail

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/emersion/go-message"
)

func GetMessageBodyHTML(db *sql.DB, messageId int64) (string, bool, error) {
	var path string
	if err := db.QueryRow("SELECT path FROM message WHERE id = ?", messageId).Scan(&path); err != nil {
		return "", false, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", path, err)
	}

	entity, err := message.Read(bytes.NewReader(raw))
	if err != nil {
		return "", false, err
	}

	var htmlBody, textBody string

	walk := func(path []int, entity *message.Entity, err error) error {
		if err != nil {
			return err
		}
		contentType, _, err := entity.Header.ContentType()
		if err != nil {
			return err
		}

		switch {
		case strings.Contains(contentType, "text/html"):
			buf := new(bytes.Buffer)
			buf.ReadFrom(entity.Body)
			htmlBody += buf.String() + "\n"
		case strings.Contains(contentType, "text/plain"):
			buf := new(bytes.Buffer)
			buf.ReadFrom(entity.Body)
			textBody += buf.String() + "\n"
		}
		return nil
	}

	if walkErr := entity.Walk(walk); walkErr != nil && htmlBody == "" && textBody == "" {
		return "", false, walkErr
	}

	if htmlBody != "" {
		return htmlBody, true, nil
	}
	return textBody, false, nil
}
