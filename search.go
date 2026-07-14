// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/kirill-scherba/keyvalembd"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ragSearchTool performs semantic search across the knowledge base and returns
// matching chunks with similarity scores. No LLM generation is performed,
// making it useful for debugging embedding quality and for other MCP tools
// that need raw search results.
func ragSearchTool(kv *keyvalembd.KeyValueEmbd) server.ServerTool {
	opt := mcp.NewTool("rag_search",
		mcp.WithDescription(`Search the RAG knowledge base for relevant chunks.
Performs semantic search and returns the most similar chunks with their scores
and text previews. No LLM generation is performed, so this tool is ideal for
debugging embedding quality or when another tool needs raw search results.`),
		mcp.WithString("query",
			mcp.Description("The search query"),
			mcp.Required(),
		),
		mcp.WithNumber("top_k",
			mcp.Description("Maximum number of results to return (default: 5, max: 20)"),
		),
	)

	return server.ServerTool{
		Tool: opt,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			mu.Lock()
			defer mu.Unlock()
			args := request.GetArguments()

			query, _ := args["query"].(string)
			if query == "" {
				return mcp.NewToolResultText("Error: query is required"), nil
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

			searchResults, err := withEmbedderRetry(ctx, func() ([]keyvalembd.SearchResult, error) {
				return kv.SearchSemantic(query, topK)
			})
			if err != nil {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Search error: %v\nTip: Ensure Ollama is running and has the embedding model installed.", err)), nil
			}

			if len(searchResults) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf("No relevant chunks found for query %q.", query)), nil
			}

			var out strings.Builder
			out.WriteString(fmt.Sprintf("Search results for %q (%d found):\n\n", query, len(searchResults)))
			for i, sr := range searchResults {
				text := sr.Text
				preview := text
				if runes := []rune(text); len(runes) > 120 {
					preview = string(runes[:120]) + "…"
				}
				out.WriteString(fmt.Sprintf("%d. [score: %.4f] %s\n   %s\n",
					i+1, sr.Score, sr.Key, preview))
			}

			return mcp.NewToolResultText(out.String()), nil
		},
	}
}
