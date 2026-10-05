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
	"path/filepath"
	"strings"
	"time"

	"github.com/kirill-scherba/rag-mcp/rag"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ragIngestTool ingests (saves) a document: chunks text, embeds, stores.
// Provide either 'text' (inline content) or 'file_path' (path to file on disk).
func ragIngestTool(store *rag.Store) server.ServerTool {
	opt := mcp.NewTool("rag_ingest",
		mcp.WithDescription(`Ingest a document into the RAG knowledge base.
Splits the text into chunks, generates embeddings for each chunk,
and stores them for semantic search.
Provide either 'text' (inline content) or 'file_path' (path to file on disk).
With EXTRACTOR_CMD configured, file_path is processed by that command;
otherwise the file is read as text.`),
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
			filePath, _ := args["file_path"].(string)
			text, _ := args["text"].(string)
			description, _ := args["description"].(string)

			if key == "" || (filePath == "" && text == "") {
				return mcp.NewToolResultText("Error: key and either text or file_path are required"), nil
			}

			var (
				stats rag.IngestStats
				err   error
			)
			if filePath != "" {
				stats, err = store.IngestFile(ctx, key, filePath, description)
			} else {
				stats, err = store.Ingest(ctx, key, text, "", description)
			}
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error ingesting %q: %v", key, err)), nil
			}

			out := fmt.Sprintf("Ingested %q: %d chunks (replaced %d)\n", key, stats.NumChunks, stats.DeletedOld)
			out += fmt.Sprintf("  description: %s\n", stats.Description)
			return mcp.NewToolResultText(out), nil
		},
	}
}

// ragIngestDirectoryTool ingests all files matching a pattern in a directory.
func ragIngestDirectoryTool(store *rag.Store) server.ServerTool {
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
				baseName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
				docKey := keyPrefix + "/" + baseName

				stats, err := store.IngestFile(ctx, docKey, filePath, "")
				if err != nil {
					fileResults = append(fileResults, fmt.Sprintf("  ❌ %s: %v", filePath, err))
					continue
				}
				totalChunks += stats.NumChunks
				fileResults = append(fileResults, fmt.Sprintf(
					"  ✅ %s → %s (%d chunks, replaced %d)", filePath, docKey, stats.NumChunks, stats.DeletedOld))
			}

			out := fmt.Sprintf("Ingested %d files (%d total chunks):\n", len(files), totalChunks)
			out += strings.Join(fileResults, "\n")
			return mcp.NewToolResultText(out), nil
		},
	}
}

// ragIngestUrlTool fetches a URL and ingests its content as a document.
func ragIngestUrlTool(store *rag.Store) server.ServerTool {
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

			stats, err := store.Ingest(ctx, docKey, text, urlStr, "")
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error ingesting %q: %v", docKey, err)), nil
			}

			out := fmt.Sprintf("Ingested %q as %s (%d chunks, replaced %d):\n", urlStr, docKey, stats.NumChunks, stats.DeletedOld)
			out += fmt.Sprintf("  description: %s\n", stats.Description)
			return mcp.NewToolResultText(out), nil
		},
	}
}
