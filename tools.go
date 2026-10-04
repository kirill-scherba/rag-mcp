// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kirill-scherba/keyvalembd"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// tools returns all MCP tools for rag-mcp.
func tools(srv *server.MCPServer, kv *keyvalembd.KeyValueEmbd, ti *textIndex) []server.ServerTool {
	return []server.ServerTool{
		ragIngestTool(kv),
		ragIngestDirectoryTool(kv),
		ragIngestUrlTool(kv),
		ragSearchTool(kv),
		ragFindTool(ti),
		ragQueryTool(srv, kv),
		ragDeleteTool(kv),
		ragListTool(kv),
	}
}

// ragFindTool performs exact keyword (SQL LIKE) search, complementing the
// semantic rag_search. It needs no embeddings and works with Ollama down.
func ragFindTool(ti *textIndex) server.ServerTool {
	opt := mcp.NewTool("rag_find",
		mcp.WithDescription(`Exact keyword search (SQL LIKE) across the knowledge base.
Complements rag_search (semantic): use it for an exact word or phrase —
names, places, quotes (e.g. "Золотая Вобла", "Шашлычная 1957"). Case-insensitive
for ASCII; a first-letter-uppercase variant is also tried for Russian.`),
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
			if ti == nil {
				return mcp.NewToolResultText("Text search is unavailable (no database connection)."), nil
			}
			args := request.GetArguments()
			keyword, _ := args["keyword"].(string)
			if keyword == "" {
				return mcp.NewToolResultText("Error: keyword is required"), nil
			}
			limit := 20
			if v, ok := args["limit"].(float64); ok {
				limit = int(v)
			}

			results, err := ti.find(keyword, limit)
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

// docEntry represents a single document for listing.
type docEntry struct {
	Key         string
	Description string
	NumChunks   int
	Stored      string
}

// collectDocs recursively collects document keys under a prefix.
func collectDocs(kv *keyvalembd.KeyValueEmbd, prefix string, out map[string]struct{}) {
	for key := range kv.List(prefix) {
		if isMetaKey(key) {
			docKey := strings.TrimSuffix(key, "/"+metaSuffix)
			out[docKey] = struct{}{}
		} else if isChunkKey(key) {
			parts := strings.Split(key, "/chunk/")
			if len(parts) > 0 {
				out[parts[0]] = struct{}{}
			}
		} else if strings.HasSuffix(key, "/") {
			collectDocs(kv, key, out)
		} else {
			out[key] = struct{}{}
		}
	}
}

// listDocs collects all document entries under a prefix.
// If prefix is empty, recursively walks all folders.
func listDocs(kv *keyvalembd.KeyValueEmbd, prefix string) []docEntry {
	docKeySet := make(map[string]struct{})
	collectDocs(kv, prefix, docKeySet)

	var entries []docEntry
	for docKey := range docKeySet {
		meta := loadMeta(kv, docKey)
		if meta != nil {
			entries = append(entries, docEntry{
				Key:         docKey,
				Description: meta.Description,
				NumChunks:   meta.NumChunks,
				Stored:      meta.Stored,
			})
		} else {
			numChunks := 0
			for k := range kv.List(docKey) {
				if isChunkKey(k) {
					numChunks++
				}
			}
			entries = append(entries, docEntry{
				Key:       docKey,
				NumChunks: numChunks,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})
	return entries
}

// formatDocDetail formats detailed info for a single document.
func formatDocDetail(kv *keyvalembd.KeyValueEmbd, e docEntry) *mcp.CallToolResult {
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
			chunkKey := fmt.Sprintf("%s/chunk/%04d", e.Key, i)
			// Load chunk value and extract text preview when available.
			textPreview := ""
			if data, err := kv.Get(chunkKey); err == nil && len(data) > 0 {
				var ch struct {
					Text string `json:"text"`
				}
				if json.Unmarshal(data, &ch) == nil && ch.Text != "" {
					runes := []rune(ch.Text)
					if len(runes) > 100 {
						textPreview = string(runes[:100]) + "…"
					} else {
						textPreview = ch.Text
					}
				}
			}
			if textPreview != "" {
				out.WriteString(fmt.Sprintf("  chunk %04d: %s\n", i, textPreview))
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
// Without arguments, shows all documents with descriptions.
// With a key prefix, shows documents under that prefix.
// With a specific document key, shows its metadata and chunks.
func ragListTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
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

			entries := listDocs(kv, prefix)

			if len(entries) == 0 {
				if prefix == "" {
					return mcp.NewToolResultText("Knowledge base is empty."), nil
				}
				return mcp.NewToolResultText(fmt.Sprintf("No documents found under '%s'.", prefix)), nil
			}

			if hasPrefix && len(entries) == 1 && entries[0].Key == prefix {
				return formatDocDetail(kv, entries[0]), nil
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
