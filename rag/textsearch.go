// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rag

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"modernc.org/sqlite"
)

// TextIndex provides exact keyword (Unicode case-insensitive) search over a
// RAG database. It owns a separate read connection so the keyvalembd handle
// stays untouched.
type TextIndex struct {
	db *sql.DB
}

// registerOnce registers the Unicode-aware helpers on the sqlite driver. The
// driver applies a registration to every connection opened afterwards.
var registerOnce sync.Once

func registerUnicodeFunctions() error {
	var err error
	registerOnce.Do(func() {
		// ucontains(haystack, needle) reports whether haystack contains needle,
		// case-insensitively for the whole Unicode range (Go strings.ToLower),
		// unlike SQLite's LIKE/lower which fold ASCII only.
		err = sqlite.RegisterDeterministicScalarFunction("ucontains", 2,
			func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
				if len(args) != 2 {
					return int64(0), nil
				}
				hay, _ := args[0].(string)
				needle, _ := args[1].(string)
				if needle == "" {
					return int64(0), nil
				}
				if strings.Contains(strings.ToLower(hay), strings.ToLower(needle)) {
					return int64(1), nil
				}
				return int64(0), nil
			})
	})
	return err
}

// OpenTextIndex opens a connection to the database for keyword search.
func OpenTextIndex(dbPath string) (*TextIndex, error) {
	if err := registerUnicodeFunctions(); err != nil {
		return nil, fmt.Errorf("register sqlite functions: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open text index: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping text index: %w", err)
	}
	return &TextIndex{db: db}, nil
}

// Close releases the read connection.
func (t *TextIndex) Close() {
	if t != nil && t.db != nil {
		_ = t.db.Close()
	}
}

// FindResult is a single keyword-search hit.
type FindResult struct {
	Key  string
	Text string
}

// Find searches keys and values with a Unicode-aware case-insensitive
// substring match (ucontains). It reads kv_data (not kv_embeddings), so it
// works whether or not embeddings were generated — e.g. with Ollama down.
// Chunk rows carry the text in their JSON value; document rows carry a
// description.
func (t *TextIndex) Find(keyword string, limit int) ([]FindResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT key, CAST(value AS TEXT) AS val FROM kv_data
		WHERE ucontains(key, ?) OR ucontains(CAST(value AS TEXT), ?)
		ORDER BY key
		LIMIT ?`

	rows, err := t.db.Query(query, keyword, keyword, limit)
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}
	defer rows.Close()

	var results []FindResult
	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, FindResult{Key: key, Text: readableValue(val)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return results, nil
}

// readableValue returns the chunk text or document description from a stored
// JSON value, falling back to the raw value.
func readableValue(val string) string {
	var m struct {
		Text        string `json:"text"`
		Description string `json:"description"`
	}
	if json.Unmarshal([]byte(val), &m) == nil {
		if m.Text != "" {
			return m.Text
		}
		if m.Description != "" {
			return m.Description
		}
	}
	return val
}
