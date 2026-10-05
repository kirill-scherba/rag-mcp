// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rag

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// setupTestStore creates a temporary RAG Store for testing.
func setupTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestMetaKey verifies the metadata key builder.
func TestMetaKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"rag/docs/foo", "rag/docs/foo/meta"},
		{"test", "test/meta"},
		{"", "/meta"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := metaKey(tt.input); got != tt.want {
				t.Errorf("metaKey(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestIsMetaKey verifies meta key pattern matching.
func TestIsMetaKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"rag/docs/foo/meta", true},
		{"rag/docs/foo/chunk/0000", false},
		{"rag/docs/foo", false},
		{"meta", false},
		{"/meta", true},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := isMetaKey(tt.key); got != tt.want {
				t.Errorf("isMetaKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

// TestIsChunkKey verifies chunk key pattern matching.
func TestIsChunkKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"rag/docs/foo/chunk/0000", true},
		{"rag/docs/foo/chunk/", true},
		{"rag/docs/foo/meta", false},
		{"rag/docs/foo", false},
		{"chunk", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := isChunkKey(tt.key); got != tt.want {
				t.Errorf("isChunkKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

// TestResultMarshal verifies JSON round-trip for Result.
func TestResultMarshal(t *testing.T) {
	original := Result{
		Key:      "test/key",
		Text:     "some text",
		Score:    0.95,
		Checksum: "abc123",
		Index:    5,
		Total:    10,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Key != original.Key || decoded.Text != original.Text || decoded.Score != original.Score ||
		decoded.Checksum != original.Checksum || decoded.Index != original.Index || decoded.Total != original.Total {
		t.Errorf("round-trip mismatch: %+v", decoded)
	}
}

// TestDocMetaMarshal verifies JSON round-trip for DocMeta.
func TestDocMetaMarshal(t *testing.T) {
	original := DocMeta{
		Description: "Test doc",
		NumChunks:   5,
		Source:      "/path/to/file",
		Stored:      "2026-01-01T00:00:00Z",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded DocMeta
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: %+v", decoded)
	}
}

// TestLoadMeta verifies loading metadata from keyvalembd.
func TestLoadMeta(t *testing.T) {
	s := setupTestStore(t)
	docKey := "rag/test/doc"

	meta := DocMeta{
		Description: "Test document",
		NumChunks:   3,
		Source:      "/path/to/source",
		Stored:      "2026-06-17T00:00:00Z",
	}
	data, _ := json.Marshal(meta)
	if _, err := s.KV().Set(metaKey(docKey), data); err != nil {
		t.Fatalf("kv.Set: %v", err)
	}

	loaded := s.loadMeta(docKey)
	if loaded == nil {
		t.Fatal("loadMeta returned nil for existing meta")
	}
	if *loaded != meta {
		t.Errorf("loaded=%+v, want %+v", *loaded, meta)
	}
}

// TestLoadMetaMissing verifies loadMeta returns nil for missing metadata.
func TestLoadMetaMissing(t *testing.T) {
	s := setupTestStore(t)
	if s.loadMeta("nonexistent/doc") != nil {
		t.Error("loadMeta should return nil for missing document")
	}
}

// TestLoadMetaInvalidJSON verifies loadMeta returns nil for invalid JSON.
func TestLoadMetaInvalidJSON(t *testing.T) {
	s := setupTestStore(t)
	docKey := "rag/test/badmeta"
	if _, err := s.KV().Set(metaKey(docKey), []byte("not json")); err != nil {
		t.Fatalf("kv.Set: %v", err)
	}
	if s.loadMeta(docKey) != nil {
		t.Error("loadMeta should return nil for invalid JSON")
	}
}

// TestDeleteOldChunks verifies deleteOldChunks cleans up entries.
func TestDeleteOldChunks(t *testing.T) {
	s := setupTestStore(t)
	docKey := "rag/test/cleanup"

	for _, key := range []string{docKey + "/chunk/0000", docKey + "/chunk/0001", docKey + "/meta"} {
		if _, err := s.KV().Set(key, []byte("data")); err != nil {
			t.Fatalf("kv.Set(%s): %v", key, err)
		}
	}

	deleted, err := s.deleteOldChunks(t.Context(), docKey)
	if err != nil {
		t.Fatalf("deleteOldChunks: %v", err)
	}
	if deleted == 0 {
		t.Error("expected some deletions")
	}
	if _, err := s.KV().Get(metaKey(docKey)); err == nil {
		t.Error("meta should be deleted")
	}
}

// TestDeleteOldChunksEmpty verifies delete on non-existent document.
func TestDeleteOldChunksEmpty(t *testing.T) {
	s := setupTestStore(t)
	deleted, err := s.deleteOldChunks(t.Context(), "nonexistent/doc")
	if err != nil {
		t.Fatalf("deleteOldChunks on empty: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deletions for empty doc, got %d", deleted)
	}
}

// TestDeleteOldChunksRespectsBoundaries verifies deletion respects docKey boundaries.
func TestDeleteOldChunksRespectsBoundaries(t *testing.T) {
	s := setupTestStore(t)
	docKey := "rag/docs/project/arch"

	for _, key := range []string{docKey + "/chunk/0000", docKey + "/chunk/0001", docKey + "/meta"} {
		if _, err := s.KV().Set(key, []byte("val")); err != nil {
			t.Fatalf("kv.Set(%s): %v", key, err)
		}
	}
	siblingKey := "rag/docs/project/other"
	if _, err := s.KV().Set(siblingKey+"/chunk/0000", []byte("sibling")); err != nil {
		t.Fatalf("kv.Set sibling: %v", err)
	}
	if _, err := s.KV().Set(siblingKey+"/meta", []byte("sibling-meta")); err != nil {
		t.Fatalf("kv.Set sibling meta: %v", err)
	}

	if _, err := s.deleteOldChunks(t.Context(), docKey); err != nil {
		t.Fatalf("deleteOldChunks: %v", err)
	}

	remaining := 0
	for range s.KV().List(siblingKey) {
		remaining++
	}
	if remaining == 0 {
		t.Error("sibling document should still exist")
	}
	if _, err := s.KV().Get(metaKey(docKey)); err == nil {
		t.Error("arch meta should be deleted")
	}
}

// TestCollectDocs verifies recursive document collection.
func TestCollectDocs(t *testing.T) {
	s := setupTestStore(t)
	docs := []string{
		"rag/docs/a/meta",
		"rag/docs/a/chunk/0000",
		"rag/docs/b/meta",
		"rag/docs/b/chunk/0000",
		"rag/docs/b/chunk/0001",
		"rag/docs/c/chunk/0000", // no meta
	}
	for _, key := range docs {
		if _, err := s.KV().Set(key, []byte("data")); err != nil {
			t.Fatalf("kv.Set(%s): %v", key, err)
		}
	}

	out := make(map[string]struct{})
	s.collectDocs("rag/docs", out)

	for _, want := range []string{"rag/docs/a", "rag/docs/b", "rag/docs/c"} {
		if _, ok := out[want]; !ok {
			t.Errorf("missing doc %q", want)
		}
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 docs, got %d: %v", len(out), out)
	}
}

// TestCollectDocsEmpty verifies collection on empty store.
func TestCollectDocsEmpty(t *testing.T) {
	s := setupTestStore(t)
	out := make(map[string]struct{})
	s.collectDocs("", out)
	if len(out) != 0 {
		t.Errorf("expected 0 docs in empty store, got %d", len(out))
	}
}

// TestListDocs verifies document listing.
func TestListDocs(t *testing.T) {
	s := setupTestStore(t)
	docs := []struct {
		key         string
		description string
		numChunks   int
	}{
		{"rag/docs/alpha", "Alpha doc", 2},
		{"rag/docs/beta", "Beta doc", 1},
	}
	for _, d := range docs {
		meta := DocMeta{Description: d.description, NumChunks: d.numChunks, Stored: "2026-01-01"}
		data, _ := json.Marshal(meta)
		if _, err := s.KV().Set(metaKey(d.key), data); err != nil {
			t.Fatalf("kv.Set meta: %v", err)
		}
		for i := 0; i < d.numChunks; i++ {
			if _, err := s.KV().Set(d.key+"/chunk/0000", []byte("chunk")); err != nil {
				t.Fatalf("kv.Set chunk: %v", err)
			}
		}
	}

	entries := s.List("rag/docs")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Key != "rag/docs/alpha" || entries[1].Key != "rag/docs/beta" {
		t.Errorf("unexpected order: %+v", entries)
	}
	if entries[0].Description != "Alpha doc" {
		t.Errorf("alpha description: got %q", entries[0].Description)
	}
}

// TestListDocsNoMeta verifies listing when metadata is missing.
func TestListDocsNoMeta(t *testing.T) {
	s := setupTestStore(t)
	if _, err := s.KV().Set("rag/docs/nometa/chunk/0000", []byte("chunk")); err != nil {
		t.Fatalf("kv.Set: %v", err)
	}
	entries := s.List("rag/docs")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Key != "rag/docs/nometa" || entries[0].NumChunks != 1 || entries[0].Description != "" {
		t.Errorf("unexpected entry: %+v", entries[0])
	}
}
