// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rag

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirill-scherba/keyvalembd"
)

func TestTextIndexFind(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Store chunk rows directly in kv_data (no embeddings needed): Find reads
	// kv_data, so it works with Ollama down.
	kv, err := keyvalembd.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	rows := []struct{ key, value string }{
		{"rag/docs/a/chunk/0000", `{"text":"Гуляли по городу и увидели вывеску Золотая Вобла."}`},
		{"rag/docs/a/chunk/0001", `{"text":"Coffee and tea are served here."}`},
		{"rag/docs/b/chunk/0000", `{"text":"Совсем другой текст."}`},
		{"rag/docs/a/meta", `{"description":"Заметки о прогулке"}`},
	}
	for _, r := range rows {
		if _, err := kv.Set(r.key, []byte(r.value)); err != nil {
			t.Fatal(err)
		}
	}
	kv.Close()

	ti, err := OpenTextIndex(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ti.Close()

	// Cyrillic keyword, exact case.
	res, err := ti.Find("Золотая Вобла", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || !strings.Contains(res[0].Text, "Золотая Вобла") {
		t.Fatalf("cyrillic search: %+v", res)
	}

	// Cyrillic is case-insensitive too (ucontains).
	for _, q := range []string{"золотая вобла", "ЗОЛОТАЯ ВОБЛА", "вОбЛа"} {
		res, err = ti.Find(q, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 {
			t.Fatalf("case-insensitive %q: %+v", q, res)
		}
	}

	// ASCII keyword is case-insensitive.
	res, err = ti.Find("COFFEE", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("ascii case-insensitive search: %+v", res)
	}

	// Document description is found too.
	res, err = ti.Find("прогулке", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || !strings.Contains(res[0].Text, "прогулке") {
		t.Fatalf("description search: %+v", res)
	}

	// No match.
	res, err = ti.Find("нет-такого-слова", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Fatalf("expected no results, got %+v", res)
	}
}
