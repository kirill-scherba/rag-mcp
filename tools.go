// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kirill-scherba/keyvalembd"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ragResult holds a single chunk result from RAG operations.
type ragResult struct {
	Key      string  `json:"key"`
	Text     string  `json:"text"`
	Score    float64 `json:"score"`
	Checksum string  `json:"checksum"`
	Index    int     `json:"index"`
	Total    int     `json:"total"`
}

// Serialize access to keyvalembd so that tools/call handlers execute
// sequentially, avoiding race conditions when multiple requests arrive
// on the same stdin stream (e.g. ingest → query → delete in one pipe).
var mu sync.Mutex

// tools returns all MCP tools for rag-mcp.
func tools(kv *keyvalembd.KeyValueEmbd) []server.ServerTool {
	return []server.ServerTool{
		ragIngestTool(kv),
		ragQueryTool(kv),
		ragDeleteTool(kv),
		ragListTool(kv),
	}
}

// ─── rag_ingest ──────────────────────────────────────────────────────────────────

// ragIngestTool ingests (saves) a document: chunks text, embeds, stores.
func ragIngestTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
	opt := mcp.NewTool("rag_ingest",
		mcp.WithDescription(`Ingest a document into the RAG knowledge base.
Splits the text into chunks, generates embeddings for each chunk,
and stores them for semantic search.`),
		mcp.WithString("key",
			mcp.Description("Document key (e.g. rag/docs/cooksy/architecture)"),
			mcp.Required(),
		),
		mcp.WithString("text",
			mcp.Description("Full document text to ingest"),
			mcp.Required(),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			mu.Lock()
			defer mu.Unlock()
			args := request.GetArguments()
			key, _ := args["key"].(string)
			text, _ := args["text"].(string)

			if key == "" || text == "" {
				return mcp.NewToolResultText("Error: key and text are required"), nil
			}

			// Chunk the text
			chunks := chunkText(text)
			if len(chunks) == 0 {
				return mcp.NewToolResultText("Error: no chunks generated from text"), nil
			}

			// Store each chunk as a separate key with embedding
			var results []string
			for i, chunk := range chunks {
				chunkKey := fmt.Sprintf("%s/chunk/%04d", key, i)

				// Create checksum for the chunk
				checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(chunk)))

				// Value stores chunk info
				val := map[string]interface{}{
					"index":    i,
					"total":    len(chunks),
					"checksum": checksum,
					"text":     chunk,
					"doc_key":  key,
					"stored":   time.Now().UTC().Format(time.RFC3339),
				}
				valJSON, _ := json.Marshal(val)

				// Store with embedding (the chunk text is used for embedding)
				info, err := kv.SetWithEmbedding(chunkKey, valJSON, chunk)
				if err != nil {
					return mcp.NewToolResultText(fmt.Sprintf(
						"Error storing chunk %d/%d: %v", i+1, len(chunks), err)), nil
				}

				results = append(results, fmt.Sprintf(
					"  chunk %d/%d: key=%s, size=%d", i+1, len(chunks), info.Checksum, info.ContentLength))
			}

			out := fmt.Sprintf("Ingested %d chunks:\n", len(chunks))
			out += strings.Join(results, "\n")
			return mcp.NewToolResultText(out), nil
		},
	}
}

// ─── rag_query ───────────────────────────────────────────────────────────────────

// ragQueryTool answers a question using RAG: semantic search + LLM generation.
func ragQueryTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
	opt := mcp.NewTool("rag_query",
		mcp.WithDescription(`Answer a question using the RAG knowledge base.
Performs semantic search across ingested documents and generates
an answer using the LLM.`),
		mcp.WithString("question",
			mcp.Description("The question to answer"),
			mcp.Required(),
		),
		mcp.WithNumber("top_k",
			mcp.Description("Number of context fragments to retrieve (default: 5, max: 20)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			mu.Lock()
			defer mu.Unlock()
			args := request.GetArguments()
			question, _ := args["question"].(string)
			if question == "" {
				return mcp.NewToolResultText("Error: question is required"), nil
			}

			topK := 5
			if v, ok := args["top_k"].(float64); ok {
				topK = int(v)
			}
			if topK > 20 {
				topK = 20
			}
			if topK <= 0 {
				topK = 5
			}

			// Semantic search for relevant chunks
			searchResults, err := kv.SearchSemantic(question, topK)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Search error: %v\nTip: Ensure Ollama is running and has the embedding model installed.", err)), nil
			}

			if len(searchResults) == 0 {
				return mcp.NewToolResultText("No relevant documents found in the knowledge base to answer the question."), nil
			}

			// Convert to ragResult format using search result fields
			var chunks []ragResult
			for _, sr := range searchResults {
				chunks = append(chunks, ragResult{
					Key:   sr.Key,
					Text:  sr.Text,
					Score: sr.Score,
				})
			}

			// Format a summary of found chunks for the response
			var chunkSummary []string
			for _, ch := range chunks {
				truncated := ch.Text
				if len(truncated) > 120 {
					truncated = truncated[:120] + "..."
				}
				chunkSummary = append(chunkSummary, fmt.Sprintf(
					"- [%.4f] %s", ch.Score, truncated))
			}

			// Build prompt and generate answer
			messages, err := buildRAGPrompt(chunks, question)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error building prompt: %v", err)), nil
			}
			answer, err := generateAnswer(messages)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error generating answer: %v", err)), nil
			}

			// Build result
			result := fmt.Sprintf("Question: %s\n\n", question)
			result += fmt.Sprintf("Answer:\n%s\n\n", answer)
			result += fmt.Sprintf("Context (%d fragments):\n", len(chunks))
			result += strings.Join(chunkSummary, "\n")

			return mcp.NewToolResultText(result), nil
		},
	}
}

// ─── rag_list ────────────────────────────────────────────────────────────────────

// ragListTool lists document keys in the knowledge base.
// Without arguments, lists all top-level document keys.
// With a key prefix, lists chunks under that prefix.
func ragListTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
	opt := mcp.NewTool("rag_list",
		mcp.WithDescription(`List document keys or chunks in the RAG knowledge base.
Without arguments, lists all top-level document keys.
With a key prefix, lists chunks under that document.`),
		mcp.WithString("key",
			mcp.Description("Optional key prefix to list (e.g. rag/docs/cooksy/architecture). Lists all top-level keys if omitted."),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			mu.Lock()
			defer mu.Unlock()
			args := request.GetArguments()

			prefix, hasPrefix := args["key"].(string)
			if !hasPrefix || prefix == "" {
				prefix = ""
			}

			var entries []string
			for key := range kv.List(prefix) {
				entries = append(entries, key)
			}

			if len(entries) == 0 {
				if prefix == "" {
					return mcp.NewToolResultText("Knowledge base is empty."), nil
				}
				return mcp.NewToolResultText(fmt.Sprintf("No entries found under '%s'.", prefix)), nil
			}

			out := fmt.Sprintf("Found %d entries:\n", len(entries))
			for _, e := range entries {
				out += fmt.Sprintf("  %s\n", e)
			}
			return mcp.NewToolResultText(out), nil
		},
	}
}

// ─── rag_delete ──────────────────────────────────────────────────────────────────

// ragDeleteTool deletes a document and all its chunks from the knowledge base.
func ragDeleteTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
	opt := mcp.NewTool("rag_delete",
		mcp.WithDescription(`Delete a document and all its chunks from the RAG knowledge base.`),
		mcp.WithString("key",
			mcp.Description("Document key to delete (e.g. rag/docs/cooksy/architecture)"),
			mcp.Required(),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			mu.Lock()
			defer mu.Unlock()
			args := request.GetArguments()
			key, _ := args["key"].(string)
			if key == "" {
				return mcp.NewToolResultText("Error: key is required"), nil
			}

			// List all chunks under this key
			deleted := 0
			for chunkKey := range kv.List(key) {
				if err := kv.Del(chunkKey); err != nil {
					return mcp.NewToolResultText(fmt.Sprintf(
						"Error deleting %s: %v", chunkKey, err)), nil
				}
				deleted++
			}

			return mcp.NewToolResultText(fmt.Sprintf(
				"Deleted document '%s' (%d chunks removed)", key, deleted)), nil
		},
	}
}