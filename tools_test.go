// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirill-scherba/rag-mcp/rag"
	"github.com/mark3labs/mcp-go/mcp"
)

// setupTestStore creates a temporary RAG store for testing.
func setupTestStore(t *testing.T) *rag.Store {
	t.Helper()
	s, err := rag.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("rag.Open: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

// TestFormatDocDetail verifies document detail formatting.
func TestFormatDocDetail(t *testing.T) {
	store := setupTestStore(t)

	if _, err := store.KV().Set("rag/docs/detail/chunk/0000", []byte("chunk0")); err != nil {
		t.Fatalf("kv.Set: %v", err)
	}
	if _, err := store.KV().Set("rag/docs/detail/chunk/0001", []byte("chunk1")); err != nil {
		t.Fatalf("kv.Set: %v", err)
	}

	entry := rag.Doc{
		Key:         "rag/docs/detail",
		Description: "Detail doc",
		NumChunks:   2,
		Stored:      "2026-01-01",
	}
	result := formatDocDetail(store, entry)
	if result == nil {
		t.Fatal("formatDocDetail returned nil")
	}
	text := result.Content[0].(mcp.TextContent).Text
	for _, want := range []string{
		"Document: rag/docs/detail",
		"Description: Detail doc",
		"Chunks: 2",
		"stored 2026-01-01",
		"Chunks:\n",
		"chunk ",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q: %s", want, text)
		}
	}
}

// TestFormatDocDetailWithChunkText verifies chunk text preview is shown.
func TestFormatDocDetailWithChunkText(t *testing.T) {
	store := setupTestStore(t)

	chunkJSON := `{"index":0,"total":1,"text":"Cooksy is a recipe sharing platform built with Go and Vuejs."}`
	if _, err := store.KV().Set("rag/docs/texttest/chunk/0000", []byte(chunkJSON)); err != nil {
		t.Fatalf("kv.Set: %v", err)
	}

	entry := rag.Doc{Key: "rag/docs/texttest", NumChunks: 1}
	text := formatDocDetail(store, entry).Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Cooksy is a recipe sharing platform") {
		t.Errorf("expected chunk text preview, got: %s", text)
	}
	if !strings.Contains(text, "chunk 0000:") {
		t.Errorf("expected chunk index with colon, got: %s", text)
	}
}

// TestFormatDocDetailNoChunks verifies output when no chunks exist.
func TestFormatDocDetailNoChunks(t *testing.T) {
	store := setupTestStore(t)
	text := formatDocDetail(store, rag.Doc{Key: "rag/docs/empty", NumChunks: 0}).Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No chunks found.") {
		t.Errorf("expected 'No chunks found.', got: %s", text)
	}
}

// TestRagDeleteToolArgumentValidation tests argument checking.
func TestRagDeleteToolArgumentValidation(t *testing.T) {
	store := setupTestStore(t)
	tool := ragDeleteTool(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "Error: key is required") {
		t.Errorf("expected key required error, got: %s", text)
	}

	req.Params.Arguments = map[string]interface{}{"key": ""}
	result, err = tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "Error: key is required") {
		t.Errorf("expected key required error for empty key, got: %s", text)
	}
}

// TestRagDeleteToolDeletesDocument tests actual deletion.
func TestRagDeleteToolDeletesDocument(t *testing.T) {
	store := setupTestStore(t)
	docKey := "rag/test/deleteme"
	if _, err := store.KV().Set(docKey+"/meta", []byte(`{"num_chunks":1}`)); err != nil {
		t.Fatalf("kv.Set: %v", err)
	}

	tool := ragDeleteTool(store)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"key": docKey}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "Deleted document") {
		t.Errorf("expected deletion confirmation, got: %s", text)
	}
}

// TestRagListToolEmptyKB verifies empty knowledge base message.
func TestRagListToolEmptyKB(t *testing.T) {
	store := setupTestStore(t)
	tool := ragListTool(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if text := result.Content[0].(mcp.TextContent).Text; text != "Knowledge base is empty." {
		t.Errorf("expected empty KB message, got: %s", text)
	}
}

// TestRagListToolUnknownPrefix verifies unknown prefix message.
func TestRagListToolUnknownPrefix(t *testing.T) {
	store := setupTestStore(t)
	tool := ragListTool(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"key": "unknown/prefix"}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "No documents found") {
		t.Errorf("expected no docs message, got: %s", text)
	}
}

// TestRagListToolListsDocuments verifies document listing.
func TestRagListToolListsDocuments(t *testing.T) {
	store := setupTestStore(t)
	docs := []struct {
		key   string
		meta  string
		chunk string
	}{
		{"rag/docs/x", `{"description":"X doc","num_chunks":1}`, "cx"},
		{"rag/docs/y", `{"description":"Y doc","num_chunks":1}`, "cy"},
	}
	for _, d := range docs {
		if _, err := store.KV().Set(d.key+"/meta", []byte(d.meta)); err != nil {
			t.Fatalf("kv.Set: %v", err)
		}
		if _, err := store.KV().Set(d.key+"/chunk/0000", []byte(d.chunk)); err != nil {
			t.Fatalf("kv.Set: %v", err)
		}
	}

	tool := ragListTool(store)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	for _, want := range []string{"Found 2 documents", "X doc", "Y doc"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in output, got: %s", want, text)
		}
	}
}

// TestRagIngestToolArgumentValidation tests required argument checks.
func TestRagIngestToolArgumentValidation(t *testing.T) {
	store := setupTestStore(t)
	tool := ragIngestTool(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"text": "hello"}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "Error: key and either text or file_path are required") {
		t.Errorf("expected key required error, got: %s", text)
	}

	req.Params.Arguments = map[string]interface{}{"key": "test/key"}
	result, err = tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "Error: key and either text or file_path are required") {
		t.Errorf("expected content required error, got: %s", text)
	}
}

// TestRagIngestDirectoryToolArgumentValidation tests required args.
func TestRagIngestDirectoryToolArgumentValidation(t *testing.T) {
	store := setupTestStore(t)
	tool := ragIngestDirectoryTool(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"dir_path": "/tmp"}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "Error: key_prefix and dir_path are required") {
		t.Errorf("expected missing args error, got: %s", text)
	}

	req.Params.Arguments = map[string]interface{}{"key_prefix": "test"}
	result, err = tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "Error: key_prefix and dir_path are required") {
		t.Errorf("expected missing args error, got: %s", text)
	}

	req.Params.Arguments = map[string]interface{}{"key_prefix": "test", "dir_path": "/nonexistent/path"}
	result, err = tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "No files matching") {
		t.Errorf("expected no files message, got: %s", text)
	}
}

// TestRagIngestUrlToolArgumentValidation tests URL requirement.
func TestRagIngestUrlToolArgumentValidation(t *testing.T) {
	store := setupTestStore(t)
	tool := ragIngestUrlTool(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "Error: url is required") {
		t.Errorf("expected URL required error, got: %s", text)
	}
}

// TestRagQueryToolArgumentValidation tests query requirement.
func TestRagQueryToolArgumentValidation(t *testing.T) {
	store := setupTestStore(t)
	tool := ragQueryTool(nil, store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := tool.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "Error: question is required") {
		t.Errorf("expected question required error, got: %s", text)
	}
}
