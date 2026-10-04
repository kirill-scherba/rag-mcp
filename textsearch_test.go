// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirill-scherba/keyvalembd"
)

func TestTextIndexFind(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Create the schema with keyvalembd, then insert chunk text directly:
	// chunk text lives in kv_embeddings, and a unit test has no Ollama to
	// generate embeddings.
	kv, err := keyvalembd.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	kv.Close()

	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows := []struct{ key, text string }{
		{"rag/docs/a/chunk/0000", "Гуляли по городу и увидели вывеску Золотая Вобла."},
		{"rag/docs/a/chunk/0001", "Coffee and tea are served here."},
		{"rag/docs/b/chunk/0000", "Совсем другой текст."},
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO kv_embeddings (key, text) VALUES (?, ?)`, r.key, r.text); err != nil {
			t.Fatal(err)
		}
	}

	ti, err := openTextIndex(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ti.Close()

	// Cyrillic keyword, exact case.
	res, err := ti.find("Золотая Вобла", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || !strings.Contains(res[0].Text, "Золотая Вобла") {
		t.Fatalf("cyrillic search: %+v", res)
	}

	// ASCII keyword is case-insensitive.
	res, err = ti.find("COFFEE", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("ascii case-insensitive search: %+v", res)
	}

	// No match.
	res, err = ti.find("нет-такого-слова", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Fatalf("expected no results, got %+v", res)
	}
}
