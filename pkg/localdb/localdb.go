package localdb

import (
	"database/sql"
	_ "embed"
	"fmt"
	"puffin/pkg/assert"
)

//go:embed schema.sql
var sqlSchema string

func ConnectSqlite(filepath string) (*sql.DB, error) {
	assert.Assert(filepath != "", "empty filepath")

	db, err := sql.Open("sqlite", filepath)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if _, err = db.Exec(sqlSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	return db, nil
}

type SearchMatch struct {
	Id      string
	Subject string
	Body    string
}

func Search(db *sql.DB, query string) ([]SearchMatch, error) {
	rows, err := db.Query(`
		SELECT
			message_fts.rowid AS id,
			highlight(message_fts, 0, '<|', '|>') AS subject_highlighted,
			snippet(message_fts, 1, '<|', '|>', '...', 10) AS body_snippet
		FROM message_fts
		WHERE message_fts MATCH ?;
	`, query)
	if err != nil {
		return nil, err
	}

	// FIXME: make this more efficient
	results := make([]SearchMatch, 0)
	for rows.Next() {
		var matchId, matchSubject, matchBody string
		if err := rows.Scan(&matchId, &matchSubject, &matchBody); err != nil {
			return nil, err
		}
		results = append(results, SearchMatch{matchId, matchSubject, matchBody})
	}

	return results, nil
}
