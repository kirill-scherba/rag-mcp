// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
func tools(srv *server.MCPServer, kv *keyvalembd.KeyValueEmbd) []server.ServerTool {
	return []server.ServerTool{
		ragIngestTool(kv),
		ragIngestDirectoryTool(kv),
		ragIngestUrlTool(kv),
		ragQueryTool(srv, kv),
		ragDeleteTool(kv),
		ragListTool(kv),
	}
}

// ─── rag_ingest ──────────────────────────────────────────────────────────────────

// ragIngestTool ingests (saves) a document: chunks text, embeds, stores.
// Provide either 'text' (inline content) or 'file_path' (path to file on disk).
func ragIngestTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
	opt := mcp.NewTool("rag_ingest",
		mcp.WithDescription(`Ingest a document into the RAG knowledge base.
Splits the text into chunks, generates embeddings for each chunk,
and stores them for semantic search.
Provide either 'text' (inline content) or 'file_path' (path to file on disk).`),
		mcp.WithString("key",
			mcp.Description("Document key (e.g. rag/docs/cooksy/architecture)"),
			mcp.Required(),
		),
		mcp.WithString("text",
			mcp.Description("Full document text to ingest (mutually exclusive with file_path)"),
		),
		mcp.WithString("file_path",
			mcp.Description("Path to a file to read and ingest (mutually exclusive with text)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			mu.Lock()
			defer mu.Unlock()
			args := request.GetArguments()
			key, _ := args["key"].(string)

			// Resolve text: file_path takes precedence over inline text
			var text string
			if filePath, ok := args["file_path"].(string); ok && filePath != "" {
				data, err := os.ReadFile(filePath)
				if err != nil {
					return mcp.NewToolResultText(fmt.Sprintf(
						"Error reading file %q: %v", filePath, err)), nil
				}
				text = string(data)
			} else if t, ok := args["text"].(string); ok {
				text = t
			}

			if key == "" || text == "" {
				return mcp.NewToolResultText("Error: key and either text or file_path are required"), nil
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

// ─── rag_ingest_directory ────────────────────────────────────────────────────────

// ragIngestDirectoryTool ingests all files matching a pattern in a directory.
func ragIngestDirectoryTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
	opt := mcp.NewTool("rag_ingest_directory",
		mcp.WithDescription(`Ingest all documents from a directory into the RAG knowledge base.
Scans the directory for matching files (default: *.md,*.txt) and ingests each one.
Document key is '<key_prefix>/<filename_without_ext>'.`),
		mcp.WithString("key_prefix",
			mcp.Description("Prefix for document keys (e.g. rag/docs/cooksy)"),
			mcp.Required(),
		),
		mcp.WithString("dir_path",
			mcp.Description("Path to directory containing documents to ingest"),
			mcp.Required(),
		),
		mcp.WithString("pattern",
			mcp.Description("Glob pattern for files (default: '*.md,*.txt'). Comma-separated for multiple patterns."),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			mu.Lock()
			defer mu.Unlock()
			args := request.GetArguments()
			keyPrefix, _ := args["key_prefix"].(string)
			dirPath, _ := args["dir_path"].(string)
			pattern, _ := args["pattern"].(string)

			if keyPrefix == "" || dirPath == "" {
				return mcp.NewToolResultText("Error: key_prefix and dir_path are required"), nil
			}

			// Default patterns if not specified
			if pattern == "" {
				pattern = "*.md,*.txt"
			}
			patterns := strings.Split(pattern, ",")
			for i := range patterns {
				patterns[i] = strings.TrimSpace(patterns[i])
			}

			// Collect matching files
			var files []string
			for _, p := range patterns {
				matches, err := filepath.Glob(filepath.Join(dirPath, p))
				if err != nil {
					return mcp.NewToolResultText(fmt.Sprintf(
						"Error matching pattern %q: %v", p, err)), nil
				}
				files = append(files, matches...)
			}

			if len(files) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf(
					"No files matching '%s' found in %s", pattern, dirPath)), nil
			}

			// Ingest each file
			var fileResults []string
			totalChunks := 0
			for _, filePath := range files {
				// Read file
				data, err := os.ReadFile(filePath)
				if err != nil {
					fileResults = append(fileResults, fmt.Sprintf("  ❌ %s: %v", filePath, err))
					continue
				}

				// Generate document key from filename (without extension)
				baseName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
				docKey := keyPrefix + "/" + baseName

				// Chunk the text
				chunks := chunkText(string(data))
				if len(chunks) == 0 {
					fileResults = append(fileResults, fmt.Sprintf("  ⚠️  %s: no chunks generated", filePath))
					continue
				}

				// Store each chunk
				for i, chunk := range chunks {
					chunkKey := fmt.Sprintf("%s/chunk/%04d", docKey, i)
					checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(chunk)))
					val := map[string]interface{}{
						"index":    i,
						"total":    len(chunks),
						"checksum": checksum,
						"text":     chunk,
						"doc_key":  docKey,
						"stored":   time.Now().UTC().Format(time.RFC3339),
					}
					valJSON, _ := json.Marshal(val)
					if _, err := kv.SetWithEmbedding(chunkKey, valJSON, chunk); err != nil {
						fileResults = append(fileResults, fmt.Sprintf("  ❌ %s: chunk %d error: %v", filePath, i+1, err))
						continue
					}
				}

				totalChunks += len(chunks)
				fileResults = append(fileResults, fmt.Sprintf("  ✅ %s → %s (%d chunks)", filePath, docKey, len(chunks)))
			}

			out := fmt.Sprintf("Ingested %d files (%d total chunks):\n", len(files), totalChunks)
			out += strings.Join(fileResults, "\n")
			return mcp.NewToolResultText(out), nil
		},
	}
}

// ─── rag_ingest_url ──────────────────────────────────────────────────────────────

// ragIngestUrlTool fetches a URL and ingests its content as a document.
func ragIngestUrlTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
	opt := mcp.NewTool("rag_ingest_url",
		mcp.WithDescription(`Fetch a URL and ingest its content into the RAG knowledge base.
Downloads the content via HTTP GET, chunks it, generates embeddings,
and stores for semantic search.
If key is empty, auto-generates from the URL path.`),
		mcp.WithString("key",
			mcp.Description("Document key (e.g. rag/docs/cooksy/architecture). Auto-generated from URL if empty."),
		),
		mcp.WithString("url",
			mcp.Description("URL to fetch and ingest"),
			mcp.Required(),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			mu.Lock()
			defer mu.Unlock()
			args := request.GetArguments()
			docKey, _ := args["key"].(string)
			urlStr, _ := args["url"].(string)

			if urlStr == "" {
				return mcp.NewToolResultText("Error: url is required"), nil
			}

			// Auto-generate key from URL if not provided
			if docKey == "" {
				parsedURL, err := url.Parse(urlStr)
				if err != nil {
					return mcp.NewToolResultText(fmt.Sprintf(
						"Error parsing URL %q: %v", urlStr, err)), nil
				}
				// Use host + path as key
				path := strings.TrimSuffix(parsedURL.Path, filepath.Ext(parsedURL.Path))
				if path == "" || path == "/" {
					path = "/index"
				}
				docKey = fmt.Sprintf("rag/web/%s%s", parsedURL.Host, path)
			}

			// Fetch the URL
			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Get(urlStr)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error fetching URL %q: %v", urlStr, err)), nil
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error fetching URL %q: HTTP %d", urlStr, resp.StatusCode)), nil
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error reading response body: %v", err)), nil
			}

			text := string(body)
			if len(text) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error: empty content from %q", urlStr)), nil
			}

			// Chunk the text
			chunks := chunkText(text)
			if len(chunks) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error: no chunks generated from %q", urlStr)), nil
			}

			// Store each chunk
			var results []string
			for i, chunk := range chunks {
				chunkKey := fmt.Sprintf("%s/chunk/%04d", docKey, i)
				checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(chunk)))
				val := map[string]interface{}{
					"index":    i,
					"total":    len(chunks),
					"checksum": checksum,
					"text":     chunk,
					"doc_key":  docKey,
					"source":   urlStr,
					"stored":   time.Now().UTC().Format(time.RFC3339),
				}
				valJSON, _ := json.Marshal(val)
				info, err := kv.SetWithEmbedding(chunkKey, valJSON, chunk)
				if err != nil {
					return mcp.NewToolResultText(fmt.Sprintf(
						"Error storing chunk %d/%d: %v", i+1, len(chunks), err)), nil
				}
				results = append(results, fmt.Sprintf(
					"  chunk %d/%d: key=%s, size=%d", i+1, len(chunks), info.Checksum, info.ContentLength))
			}

			out := fmt.Sprintf("Ingested %q as %s (%d chunks):\n", urlStr, docKey, len(chunks))
			out += strings.Join(results, "\n")
			return mcp.NewToolResultText(out), nil
		},
	}
}

// ─── rag_query ───────────────────────────────────────────────────────────────────

// ragQueryTool answers a question using RAG: semantic search + LLM generation.
// Sends progress notifications to the client so the user can see real-time
// status updates via the MCP progress bar.
func ragQueryTool(srv *server.MCPServer, kv *keyvalembd.KeyValueEmbd) server.ServerTool {
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

			// Extract progressToken from request metadata (if provided by client)
			var progressToken mcp.ProgressToken
			if request.Params.Meta != nil {
				progressToken = request.Params.Meta.ProgressToken
			}

			// Helper to send a progress notification (fire-and-forget, errors ignored).
			sendProgress := func(progress, total float64, message string) {
				if srv == nil || progressToken == nil {
					return
				}
				params := map[string]any{
					"progressToken": progressToken,
					"progress":      progress,
					"total":         total,
					"message":       message,
				}
				_ = srv.SendNotificationToClient(ctx, "notifications/progress", params)
			}

			// Stage 1: Semantic search
			sendProgress(0, 100, "🔍 Searching knowledge base...")
			searchResults, err := kv.SearchSemantic(question, topK)
			if err != nil {
				sendProgress(100, 100, "❌ Search failed")
				return mcp.NewToolResultText(fmt.Sprintf(
					"Search error: %v\nTip: Ensure Ollama is running and has the embedding model installed.", err)), nil
			}

			if len(searchResults) == 0 {
				sendProgress(100, 100, "❌ No relevant documents found")
				return mcp.NewToolResultText("No relevant documents found in the knowledge base to answer the question."), nil
			}

			sendProgress(30, 100, fmt.Sprintf("📄 Found %d relevant fragments, generating answer...", len(searchResults)))

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
				truncated := []rune(ch.Text)
				if len(truncated) > 120 {
					truncated = truncated[:120]
				}
				chunkSummary = append(chunkSummary, fmt.Sprintf(
					"- [%.4f] %s", ch.Score, string(truncated)+"..."))
			}

			// Build prompt and generate answer
			messages, err := buildRAGPrompt(chunks, question)
			if err != nil {
				sendProgress(100, 100, "❌ Failed to build prompt")
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error building prompt: %v", err)), nil
			}

			// Progress callback: sends progress updates with a rolling average
			// of message tokens so the user can see the answer being built.
			const totalProgress = 100.0
			const progressStart = 30.0
			const progressEnd = 95.0

			tokenCount := 0
			progressFn := func(token string) {
				tokenCount++
				// Map token count to progress range [30..95]
				// Use a logarithmic-like scale: first tokens advance faster
				p := progressStart + (progressEnd-progressStart)*
					float64(tokenCount)/float64(tokenCount+50)
				if p > progressEnd {
					p = progressEnd
				}
				sendProgress(p, totalProgress, fmt.Sprintf("💬 Generating answer... (token %d)", tokenCount))
			}

			answer, err := generateAnswerStream(messages, progressFn)
			if err != nil {
				sendProgress(100, 100, "❌ Answer generation failed")
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error generating answer: %v", err)), nil
			}

			sendProgress(100, 100, "✅ Answer generated")

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