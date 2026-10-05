// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/kirill-scherba/rag-mcp/rag"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// tools returns all MCP tools for rag-mcp. The server is a thin wrapper over
// the rag.Store engine.
func tools(srv *server.MCPServer, store *rag.Store) []server.ServerTool {
	return []server.ServerTool{
		ragIngestTool(store),
		ragIngestDirectoryTool(store),
		ragIngestUrlTool(store),
		ragSearchTool(store),
		ragFindTool(store),
		ragQueryTool(srv, store),
		ragDeleteTool(store),
		ragListTool(store),
	}
}

// ragFindTool performs exact keyword (SQL LIKE) search, complementing the
// semantic rag_search. It needs no embeddings and works with Ollama down.
func ragFindTool(store *rag.Store) server.ServerTool {
	opt := mcp.NewTool("rag_find",
		mcp.WithDescription(`Exact keyword search across the knowledge base.
Complements rag_search (semantic): use it for an exact word or phrase —
names, places, quotes (e.g. "Золотая Вобла", "Шашлычная 1957").
Case-insensitive for the whole Unicode range.`),
		mcp.WithString("keyword",
			mcp.Description("Keyword or phrase to search for"),
			mcp.Required(),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results (default: 20, max: 100)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			mu.Lock()
			defer mu.Unlock()
			args := request.GetArguments()
			keyword, _ := args["keyword"].(string)
			if keyword == "" {
				return mcp.NewToolResultText("Error: keyword is required"), nil
			}
			limit := 20
			if v, ok := args["limit"].(float64); ok {
				limit = int(v)
			}

			results, err := store.Find(keyword, limit)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error: %v", err)), nil
			}
			if len(results) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf("No results found for keyword: %s", keyword)), nil
			}

			var out strings.Builder
			fmt.Fprintf(&out, "Found %d result(s) for %q:\n\n", len(results), keyword)
			for i, r := range results {
				preview := []rune(r.Text)
				if len(preview) > 160 {
					preview = preview[:160]
				}
				fmt.Fprintf(&out, "%d. %s\n   %s\n\n", i+1, r.Key, string(preview))
			}
			return mcp.NewToolResultText(out.String()), nil
		},
	}
}

// formatDocDetail formats detailed info for a single document.
func formatDocDetail(store *rag.Store, e rag.Doc) *mcp.CallToolResult {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("Document: %s\n", e.Key))
	if e.Description != "" {
		out.WriteString(fmt.Sprintf("Description: %s\n", e.Description))
	}
	out.WriteString(fmt.Sprintf("Chunks: %d", e.NumChunks))
	if e.Stored != "" {
		out.WriteString(fmt.Sprintf(", stored %s", e.Stored))
	}
	out.WriteString("\n\n")

	if e.NumChunks > 0 {
		out.WriteString("Chunks:\n")
		for i := 0; i < e.NumChunks; i++ {
			text := store.ChunkText(e.Key, i)
			if text != "" {
				runes := []rune(text)
				if len(runes) > 100 {
					text = string(runes[:100]) + "…"
				}
				out.WriteString(fmt.Sprintf("  chunk %04d: %s\n", i, text))
			} else {
				out.WriteString(fmt.Sprintf("  chunk %04d\n", i))
			}
		}
	} else {
		out.WriteString("No chunks found.\n")
	}

	return mcp.NewToolResultText(out.String())
}

// ragListTool lists documents in the knowledge base.
func ragListTool(store *rag.Store) server.ServerTool {
	opt := mcp.NewTool("rag_list",
		mcp.WithDescription(`List documents in the RAG knowledge base.
Without arguments, lists all documents with descriptions.
With a key prefix, lists documents under that prefix.
With a specific document key, shows its metadata and chunks.`),
		mcp.WithString("key",
			mcp.Description("Optional key prefix or document key (e.g. rag/docs/cooksy or rag/docs/cooksy/architecture). Lists all documents if omitted."),
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

			entries := store.List(prefix)

			if len(entries) == 0 {
				if prefix == "" {
					return mcp.NewToolResultText("Knowledge base is empty."), nil
				}
				return mcp.NewToolResultText(fmt.Sprintf("No documents found under '%s'.", prefix)), nil
			}

			if hasPrefix && len(entries) == 1 && entries[0].Key == prefix {
				return formatDocDetail(store, entries[0]), nil
			}

			out := fmt.Sprintf("Found %d documents:\n", len(entries))
			for _, e := range entries {
				if e.Description != "" {
					out += fmt.Sprintf("  %s — %s (%d chunks, stored %s)\n",
						e.Key, e.Description, e.NumChunks, e.Stored)
				} else {
					out += fmt.Sprintf("  %s (%d chunks)\n", e.Key, e.NumChunks)
				}
			}
			return mcp.NewToolResultText(out), nil
		},
	}
}

// ragDeleteTool deletes a document and all its chunks from the knowledge base.
func ragDeleteTool(store *rag.Store) server.ServerTool {
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

			deleted, err := store.Delete(key)
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Error deleting %s: %v", key, err)), nil
			}

			return mcp.NewToolResultText(fmt.Sprintf(
				"Deleted document '%s' (%d chunks removed)", key, deleted)), nil
		},
	}
}
