// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kirill-scherba/keyvalembd"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

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
		mcp.WithString("description",
			mcp.Description("Short description of the document (auto-generated from text if empty)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			mu.Lock()
			defer mu.Unlock()
			args := request.GetArguments()
			key, _ := args["key"].(string)

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

			description, _ := args["description"].(string)
			if description == "" {
				description = generateDescription(text, 150)
			}

			chunks := chunkTextSemantic(text)
			if len(chunks) == 0 {
				return mcp.NewToolResultText("Error: no chunks generated from text"), nil
			}

			deletedOld, err := deleteOldChunks(ctx, kv, key)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error deleting old chunks for %q: %v", key, err)), nil
			}

			results, err := storeChunks(ctx, kv, key, chunks, "")
			if err != nil {
				return mcp.NewToolResultText(err.Error()), nil
			}

			if err := storeMeta(ctx, kv, key, docMeta{
				Description: description,
				NumChunks:   len(chunks),
				Stored:      time.Now().UTC().Format(time.RFC3339),
			}); err != nil {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Warning: chunks stored but metadata save failed: %v", err)), nil
			}

			out := fmt.Sprintf("Ingested %d chunks (replaced %d old):\n", len(chunks), deletedOld)
			out += fmt.Sprintf("  meta: %s\n", metaKey(key))
			out += fmt.Sprintf("  description: %s\n", description)
			out += strings.Join(results, "\n")
			return mcp.NewToolResultText(out), nil
		},
	}
}

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

			if pattern == "" {
				pattern = "*.md,*.txt"
			}
			patterns := strings.Split(pattern, ",")
			for i := range patterns {
				patterns[i] = strings.TrimSpace(patterns[i])
			}

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

			var fileResults []string
			totalChunks := 0
			for _, filePath := range files {
				data, err := os.ReadFile(filePath)
				if err != nil {
					fileResults = append(fileResults, fmt.Sprintf("  ❌ %s: %v", filePath, err))
					continue
				}

				baseName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
				docKey := keyPrefix + "/" + baseName

				chunks := chunkTextSemantic(string(data))
				if len(chunks) == 0 {
					fileResults = append(fileResults, fmt.Sprintf("  ⚠️  %s: no chunks generated", filePath))
					continue
				}

				deletedOld, err := deleteOldChunks(ctx, kv, docKey)
				if err != nil {
					fileResults = append(fileResults, fmt.Sprintf("  ❌ %s: error deleting old chunks: %v", filePath, err))
					continue
				}

				description := generateDescription(string(data), 150)

				_, err = storeChunks(ctx, kv, docKey, chunks, filePath)
				if err != nil {
					fileResults = append(fileResults, fmt.Sprintf("  ❌ %s: %v", filePath, err))
					continue
				}

				if err := storeMeta(ctx, kv, docKey, docMeta{
					Description: description,
					NumChunks:   len(chunks),
					Source:      filePath,
					Stored:      time.Now().UTC().Format(time.RFC3339),
				}); err != nil {
					fileResults = append(fileResults, fmt.Sprintf("  ⚠️  %s: meta save failed: %v", filePath, err))
					continue
				}

				totalChunks += len(chunks)
				fileResults = append(fileResults, fmt.Sprintf("  ✅ %s → %s (%d chunks, replaced %d)", filePath, docKey, len(chunks), deletedOld))
			}

			out := fmt.Sprintf("Ingested %d files (%d total chunks):\n", len(files), totalChunks)
			out += strings.Join(fileResults, "\n")
			return mcp.NewToolResultText(out), nil
		},
	}
}

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

			if docKey == "" {
				parsedURL, err := url.Parse(urlStr)
				if err != nil {
					return mcp.NewToolResultText(fmt.Sprintf(
						"Error parsing URL %q: %v", urlStr, err)), nil
				}
				path := strings.TrimSuffix(parsedURL.Path, filepath.Ext(parsedURL.Path))
				if path == "" || path == "/" {
					path = "/index"
				}
				docKey = fmt.Sprintf("rag/web/%s%s", parsedURL.Host, path)
			}

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

			chunks := chunkTextSemantic(text)
			if len(chunks) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error: no chunks generated from %q", urlStr)), nil
			}

			deletedOld, err := deleteOldChunks(ctx, kv, docKey)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error deleting old chunks for %q: %v", docKey, err)), nil
			}

			description := generateDescription(text, 150)

			results, err := storeChunks(ctx, kv, docKey, chunks, urlStr)
			if err != nil {
				return mcp.NewToolResultText(err.Error()), nil
			}

			if err := storeMeta(ctx, kv, docKey, docMeta{
				Description: description,
				NumChunks:   len(chunks),
				Source:      urlStr,
				Stored:      time.Now().UTC().Format(time.RFC3339),
			}); err != nil {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Warning: chunks stored but metadata save failed: %v", err)), nil
			}

			out := fmt.Sprintf("Ingested %q as %s (%d chunks, replaced %d):\n", urlStr, docKey, len(chunks), deletedOld)
			out += fmt.Sprintf("  meta: %s\n", metaKey(docKey))
			out += fmt.Sprintf("  description: %s\n", description)
			out += strings.Join(results, "\n")
			return mcp.NewToolResultText(out), nil
		},
	}
}
