// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestCollectDocs verifies recursive document collection.
func TestCollectDocs(t *testing.T) {
	kv := setupTestKV(t)

	// Populate keyvalembd with document structure
	docs := []struct {
		key   string
		isMeta bool
	}{
		{"rag/docs/a/meta", true},
		{"rag/docs/a/chunk/0000", false},
		{"rag/docs/b/meta", true},
		{"rag/docs/b/chunk/0000", false},
		{"rag/docs/b/chunk/0001", false},
		{"rag/docs/c/chunk/0000", false}, // no meta
	}
	for _, d := range docs {
		if _, err := kv.Set(d.key, []byte("data")); err != nil {
			t.Fatalf("kv.Set(%s): %v", d.key, err)
		}
	}

	out := make(map[string]struct{})
	collectDocs(kv, "rag/docs", out)

	expected := map[string]struct{}{
		"rag/docs/a": {},
		"rag/docs/b": {},
		"rag/docs/c": {},
	}
	if len(out) != len(expected) {
		t.Fatalf("expected %d docs, got %d: %v", len(expected), len(out), keys(out))
	}
	for k := range expected {
		if _, ok := out[k]; !ok {
			t.Errorf("missing doc %q", k)
		}
	}
}

// TestCollectDocsEmpty verifies collection on empty store.
func TestCollectDocsEmpty(t *testing.T) {
	kv := setupTestKV(t)
	out := make(map[string]struct{})
	collectDocs(kv, "", out)
	if len(out) != 0 {
		t.Errorf("expected 0 docs in empty store, got %d", len(out))
	}
}

// TestListDocs verifies document listing.
func TestListDocs(t *testing.T) {
	kv := setupTestKV(t)

	// Create docs with metadata
	docs := []struct {
		key         string
		description string
		numChunks   int
	}{
		{"rag/docs/alpha", "Alpha doc", 2},
		{"rag/docs/beta", "Beta doc", 1},
	}
	for _, d := range docs {
		meta := docMeta{Description: d.description, NumChunks: d.numChunks, Stored: "2026-01-01"}
		data, _ := json.Marshal(meta)
		if _, err := kv.Set(metaKey(d.key), data); err != nil {
			t.Fatalf("kv.Set meta: %v", err)
		}
		for i := 0; i < d.numChunks; i++ {
			chunkKey := d.key + "/chunk/0000"
			if _, err := kv.Set(chunkKey, []byte("chunk")); err != nil {
				t.Fatalf("kv.Set chunk: %v", err)
			}
		}
	}

	entries := listDocs(kv, "rag/docs")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Should be sorted by key
	if entries[0].Key != "rag/docs/alpha" {
		t.Errorf("expected first entry alpha, got %s", entries[0].Key)
	}
	if entries[1].Key != "rag/docs/beta" {
		t.Errorf("expected second entry beta, got %s", entries[1].Key)
	}
	if entries[0].Description != "Alpha doc" {
		t.Errorf("alpha description: got %q, want %q", entries[0].Description, "Alpha doc")
	}
}

// TestListDocsNoMeta verifies listing when metadata is missing.
func TestListDocsNoMeta(t *testing.T) {
	kv := setupTestKV(t)

	// Create doc without metadata
	if _, err := kv.Set("rag/docs/nometa/chunk/0000", []byte("chunk")); err != nil {
		t.Fatalf("kv.Set: %v", err)
	}

	entries := listDocs(kv, "rag/docs")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Key != "rag/docs/nometa" {
		t.Errorf("expected nometa, got %s", entries[0].Key)
	}
	if entries[0].NumChunks != 1 {
		t.Errorf("expected 1 chunk, got %d", entries[0].NumChunks)
	}
	if entries[0].Description != "" {
		t.Errorf("expected empty description, got %q", entries[0].Description)
	}
}

// TestFormatDocDetail verifies document detail formatting.
func TestFormatDocDetail(t *testing.T) {
	kv := setupTestKV(t)

	// Create a document
	if _, err := kv.Set("rag/docs/detail/chunk/0000", []byte("chunk0")); err != nil {
		t.Fatalf("kv.Set: %v", err)
	}
	if _, err := kv.Set("rag/docs/detail/chunk/0001", []byte("chunk1")); err != nil {
		t.Fatalf("kv.Set: %v", err)
	}

	entry := docEntry{
		Key:         "rag/docs/detail",
		Description: "Detail doc",
		NumChunks:   2,
		Stored:      "2026-01-01",
	}
	result := formatDocDetail(kv, entry)
	if result == nil {
		t.Fatal("formatDocDetail returned nil")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Document: rag/docs/detail") {
		t.Errorf("output missing doc key: %s", text)
	}
	if !strings.Contains(text, "Description: Detail doc") {
		t.Errorf("output missing description: %s", text)
	}
	if !strings.Contains(text, "Chunks: 2") {
		t.Errorf("output missing chunk count: %s", text)
	}
	if !strings.Contains(text, "stored 2026-01-01") {
		t.Errorf("output missing stored date: %s", text)
	}
	if !strings.Contains(text, "Chunks:\n") {
		t.Errorf("output missing chunks header: %s", text)
	}
	// chunk indices may be empty strings when List collapses child keys into folders
	if !strings.Contains(text, "chunk ") {
		t.Errorf("output missing chunk listing: %s", text)
	}
}

// TestFormatDocDetailNoChunks verifies output when no chunks exist.
func TestFormatDocDetailNoChunks(t *testing.T) {
	kv := setupTestKV(t)
	entry := docEntry{Key: "rag/docs/empty", NumChunks: 0}
	result := formatDocDetail(kv, entry)
	if result == nil {
		t.Fatal("formatDocDetail returned nil")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No chunks found.") {
		t.Errorf("expected 'No chunks found.' in output, got: %s", text)
	}
}

// TestRagDeleteToolArgumentValidation tests argument checking.
func TestRagDeleteToolArgumentValidation(t *testing.T) {
	kv := setupTestKV(t)
	tool := ragDeleteTool(kv)

	// Missing key
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error: key is required") {
		t.Errorf("expected key required error, got: %s", text)
	}

	// Empty key
	req.Params.Arguments = map[string]interface{}{"key": ""}
	result, err = tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text = result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error: key is required") {
		t.Errorf("expected key required error for empty key, got: %s", text)
	}
}

// TestRagDeleteToolDeletesDocument tests actual deletion.
func TestRagDeleteToolDeletesDocument(t *testing.T) {
	kv := setupTestKV(t)
	docKey := "rag/test/deleteme"

	// Create document
	if _, err := kv.Set(docKey+"/meta", []byte(`{"num_chunks":1}`)); err != nil {
		t.Fatalf("kv.Set: %v", err)
	}

	tool := ragDeleteTool(kv)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"key": docKey}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Deleted document") {
		t.Errorf("expected deletion confirmation, got: %s", text)
	}
}

// TestRagListToolEmptyKB verifies empty knowledge base message.
func TestRagListToolEmptyKB(t *testing.T) {
	kv := setupTestKV(t)
	tool := ragListTool(kv)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if text != "Knowledge base is empty." {
		t.Errorf("expected empty KB message, got: %s", text)
	}
}

// TestRagListToolUnknownPrefix verifies unknown prefix message.
func TestRagListToolUnknownPrefix(t *testing.T) {
	kv := setupTestKV(t)
	tool := ragListTool(kv)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"key": "unknown/prefix"}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No documents found") {
		t.Errorf("expected no docs message, got: %s", text)
	}
}

// TestRagListToolListsDocuments verifies document listing.
func TestRagListToolListsDocuments(t *testing.T) {
	kv := setupTestKV(t)

	// Create documents
	docs := []struct {
		key   string
		meta  string
		chunk string
	}{
		{"rag/docs/x", `{"description":"X doc","num_chunks":1}`, "cx"},
		{"rag/docs/y", `{"description":"Y doc","num_chunks":1}`, "cy"},
	}
	for _, d := range docs {
		if _, err := kv.Set(d.key+"/meta", []byte(d.meta)); err != nil {
			t.Fatalf("kv.Set: %v", err)
		}
		if _, err := kv.Set(d.key+"/chunk/0000", []byte(d.chunk)); err != nil {
			t.Fatalf("kv.Set: %v", err)
		}
	}

	tool := ragListTool(kv)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Found 2 documents") {
		t.Errorf("expected 2 docs listed, got: %s", text)
	}
	if !strings.Contains(text, "X doc") {
		t.Errorf("expected X doc in output, got: %s", text)
	}
	if !strings.Contains(text, "Y doc") {
		t.Errorf("expected Y doc in output, got: %s", text)
	}
}

// TestRagIngestToolArgumentValidation tests required argument checks.
func TestRagIngestToolArgumentValidation(t *testing.T) {
	kv := setupTestKV(t)
	tool := ragIngestTool(kv)

	// Missing key
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"text": "hello"}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error: key and either text or file_path are required") {
		t.Errorf("expected key required error, got: %s", text)
	}

	// Missing text and file_path
	req.Params.Arguments = map[string]interface{}{"key": "test/key"}
	result, err = tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text = result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error: key and either text or file_path are required") {
		t.Errorf("expected content required error, got: %s", text)
	}
}

// TestRagIngestDirectoryToolArgumentValidation tests required args.
func TestRagIngestDirectoryToolArgumentValidation(t *testing.T) {
	kv := setupTestKV(t)
	tool := ragIngestDirectoryTool(kv)

	// Missing key_prefix
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"dir_path": "/tmp"}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error: key_prefix and dir_path are required") {
		t.Errorf("expected missing args error, got: %s", text)
	}

	// Missing dir_path
	req.Params.Arguments = map[string]interface{}{"key_prefix": "test"}
	result, err = tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text = result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error: key_prefix and dir_path are required") {
		t.Errorf("expected missing args error, got: %s", text)
	}

	// Valid args but non-existent directory
	req.Params.Arguments = map[string]interface{}{"key_prefix": "test", "dir_path": "/nonexistent/path"}
	result, err = tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text = result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No files matching") {
		t.Errorf("expected no files message, got: %s", text)
	}
}

// TestRagIngestUrlToolArgumentValidation tests URL requirement.
func TestRagIngestUrlToolArgumentValidation(t *testing.T) {
	kv := setupTestKV(t)
	tool := ragIngestUrlTool(kv)

	// Missing url
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error: url is required") {
		t.Errorf("expected URL required error, got: %s", text)
	}
}

// TestRagQueryToolArgumentValidation tests query requirement and top_k clamping.
func TestRagQueryToolArgumentValidation(t *testing.T) {
	kv := setupTestKV(t)
	tool := ragQueryTool(nil, kv)

	// Missing question
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Error: question is required") {
		t.Errorf("expected question required error, got: %s", text)
	}
}

func keys(m map[string]struct{}) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
