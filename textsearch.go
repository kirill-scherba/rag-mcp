// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, same as keyvalembd
)

// textIndex provides exact keyword (SQL LIKE) search over the same database,
// complementing semantic search. It owns a separate read connection so the
// keyvalembd handle stays untouched.
type textIndex struct {
	db *sql.DB
}

// openTextIndex opens a connection to the database for keyword search.
func openTextIndex(dbPath string) (*textIndex, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open text index: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping text index: %w", err)
	}
	return &textIndex{db: db}, nil
}

// Close releases the read connection.
func (t *textIndex) Close() {
	if t != nil && t.db != nil {
		_ = t.db.Close()
	}
}

// findResult is a single keyword-search hit.
type findResult struct {
	Key  string
	Text string
}

// find searches keys and values with SQL LIKE. SQLite LIKE is case-insensitive
// for ASCII but case-sensitive for Unicode (Russian), so a variant with the
// first letter uppercased is searched too — the same approach as memory_find.
func (t *textIndex) find(keyword string, limit int) ([]findResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	variants := []string{keyword}
	if r := []rune(keyword); len(r) > 0 {
		upper := string(append([]rune{uppercaseRune(r[0])}, r[1:]...))
		if upper != keyword {
			variants = append(variants, upper)
		}
	}

	clauses := make([]string, 0, len(variants)*2)
	metaClauses := make([]string, 0, len(variants)*2)
	args := make([]any, 0, len(variants)*4+1)
	for _, v := range variants {
		p := "%" + v + "%"
		clauses = append(clauses, "text LIKE ?", "key LIKE ?")
		metaClauses = append(metaClauses, "key LIKE ?", "CAST(value AS TEXT) LIKE ?")
		args = append(args, p, p)
	}
	for _, v := range variants {
		p := "%" + v + "%"
		args = append(args, p, p)
	}
	args = append(args, limit)

	// Chunk text lives in kv_embeddings (plain text); document descriptions
	// live in kv_data under "/meta" keys.
	query := fmt.Sprintf(`
		SELECT key, text AS val FROM kv_embeddings
		WHERE %s
		UNION ALL
		SELECT key, CAST(value AS TEXT) AS val FROM kv_data
		WHERE key LIKE '%%/meta' AND (%s)
		ORDER BY key
		LIMIT ?`, strings.Join(clauses, " OR "), strings.Join(metaClauses, " OR "))

	rows, err := t.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}
	defer rows.Close()

	var results []findResult
	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, findResult{Key: key, Text: val})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return results, nil
}

// uppercaseRune uppercases a single rune without importing unicode tables at
// call sites.
func uppercaseRune(r rune) rune {
	return []rune(strings.ToUpper(string(r)))[0]
}
