// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirill-scherba/keyvalembd"
)

// TestChunkText tests the chunking logic with various inputs.
func TestChunkText(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMin int // minimum expected chunks
	}{
		{
			name:    "empty string",
			input:   "",
			wantMin: 0,
		},
		{
			name:    "single paragraph",
			input:   "This is a single paragraph of text that should be long enough to form one chunk.",
			wantMin: 1,
		},
		{
			name: "multiple paragraphs",
			input: `First paragraph about Cooksy platform features and capabilities.

Second paragraph discussing architecture and design decisions.

Third paragraph covering deployment and configuration options.`,
			wantMin: 1,
		},
		{
			name:    "short text",
			input:   "Short.",
			wantMin: 0,
		},
		{
			name:    "windows line endings",
			input:   "Line one.\r\n\r\nLine two.",
			wantMin: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkTextSemantic(tt.input)
			if len(got) < tt.wantMin {
				t.Errorf("chunkTextSemantic() = %d chunks, want >= %d", len(got), tt.wantMin)
			}
			// Verify no extraneous whitespace in chunks
			for i, c := range got {
				if c != strings.TrimSpace(c) {
					t.Errorf("chunk %d has leading/trailing whitespace: %q", i, c)
				}
			}
		})
	}
}

// TestRAGIntegration is an integration test that verifies the full RAG pipeline:
//
//  1. Ingest a document (chunk → embed → store)
//  2. Query the knowledge base (semantic search + LLM generation)
//  3. Delete the document
//
// This test requires Ollama to be running with the embedding and LLM models.
func TestRAGIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !isOllamaReachable() {
		t.Skip("Ollama is not reachable at " + getOllamaBaseURL())
	}

	// Temporary database
	dbPath := filepath.Join(t.TempDir(), "rag-test.db")
	kv, err := keyvalembd.New(dbPath)
	if err != nil {
		t.Fatalf("keyvalembd.New: %v", err)
	}
	defer kv.Close()

	docKey := "rag/test/cooksy"
	docText := `Cooksy is a modern recipe sharing platform built with Go and Vuejs.
It allows users to create, share, and discover recipes from around the world.
The platform features a clean UI, powerful search, and social features like
following other chefs and commenting on recipes.

Key features include ingredient scaling, dietary filters, meal planning,
and step-by-step cooking modes with timers. Cooksy uses a microservices
architecture with Go backend services communicating via gRPC.`

	// ─── Ingest ────────────────────────────────────────────────────────────────

	t.Log("📥 Ingesting document...")
	chunks := chunkTextSemantic(docText)
	if len(chunks) == 0 {
		t.Fatal("no chunks generated from document")
	}
	t.Logf("   Generated %d chunks", len(chunks))

	for i, chunk := range chunks {
		chunkKey := fmt.Sprintf("%s/chunk/%04d", docKey, i)
		checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(chunk)))
		val := map[string]interface{}{
			"index":    i,
			"total":    len(chunks),
			"checksum": checksum,
			"text":     chunk,
			"doc_key":  docKey,
		}
		valJSON, _ := json.Marshal(val)
		info, err := kv.SetWithEmbedding(chunkKey, valJSON, chunk)
		if err != nil {
			t.Fatalf("SetWithEmbedding chunk %d/%d: %v", i+1, len(chunks), err)
		}
		t.Logf("   Stored chunk %d/%d: key=%s, size=%d", i+1, len(chunks), info.Checksum, info.ContentLength)
	}

	// Verify storage — use the chunk prefix (List is S3-style)
	chunkPrefix := docKey + "/chunk/"
	listed := 0
	for range kv.List(chunkPrefix) {
		listed++
	}
	if listed != len(chunks) {
		t.Errorf("List returned %d entries, expected %d", listed, len(chunks))
	}

	// ─── Query ─────────────────────────────────────────────────────────────────

	t.Log("🔍 Querying knowledge base...")
	question := "What is Cooksy and what are its key features?"
	searchResults, err := kv.SearchSemantic(question, 3)
	if err != nil {
		t.Fatalf("SearchSemantic: %v", err)
	}
	t.Logf("   Found %d relevant chunks", len(searchResults))

	if len(searchResults) == 0 {
		t.Fatal("no search results returned")
	}

	for i, sr := range searchResults {
		t.Logf("   Result %d: score=%.4f, key=%s", i+1, sr.Score, sr.Key)
		if sr.Score <= 0 {
			t.Errorf("Result %d has non-positive score: %.4f", i+1, sr.Score)
		}
	}

	// Generate answer via LLM
	var chunksForPrompt []ragResult
	for _, sr := range searchResults {
		chunksForPrompt = append(chunksForPrompt, ragResult{
			Key:   sr.Key,
			Text:  sr.Text,
			Score: sr.Score,
		})
	}

	t.Log("🤖 Generating answer via LLM...")
	messages, err := buildRAGPrompt(chunksForPrompt, question, "creative")
	if err != nil {
		t.Fatalf("buildRAGPrompt: %v", err)
	}

	answer, err := generateAnswerStreamWithOptions(messages, GenerateAnswerOptions{})
	if err != nil {
		t.Fatalf("generateAnswerStreamWithOptions: %v", err)
	}
	t.Logf("   Answer: %s", answer)

	if answer == "" {
		t.Error("generated answer is empty")
	}

	// ─── Delete ────────────────────────────────────────────────────────────────

	t.Log("🗑️ Deleting document...")
	deleted := 0
	for chunkKey := range kv.List(chunkPrefix) {
		if err := kv.Del(chunkKey); err != nil {
			t.Fatalf("kv.Del(%s): %v", chunkKey, err)
		}
		deleted++
	}
	t.Logf("   Deleted %d chunks", deleted)

	if deleted != len(chunks) {
		t.Errorf("Deleted %d chunks, expected %d", deleted, len(chunks))
	}

	// Verify empty
	remaining := 0
	for range kv.List(chunkPrefix) {
		remaining++
	}
	if remaining != 0 {
		t.Errorf("Expected 0 remaining, got %d", remaining)
	}

	t.Log("✅ RAG integration test PASSED")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────────

// isOllamaReachable checks if Ollama is running by pinging its API.
func isOllamaReachable() bool {
	_, err := http.Get(getOllamaBaseURL())
	return err == nil
}

// getOllamaBaseURL returns the Ollama base URL from env or default.
func getOllamaBaseURL() string {
	if u := os.Getenv("OLLAMA_BASE_URL"); u != "" {
		return u
	}
	return ollamaBaseURL
}
