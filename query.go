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
		mcp.WithString("style",
			mcp.Description("Answer style: 'creative' (default) for free-form explanation, 'strict' for exact copy-paste"),
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

			hasProgressToken := request.Params.Meta != nil && request.Params.Meta.ProgressToken != nil

			useStream := false
			switch clientMode {
			case ClientModeStream:
				useStream = true
			case ClientModeAuto:
				useStream = hasProgressToken
			}

			isStreamMode := useStream

			var progressToken mcp.ProgressToken
			if request.Params.Meta != nil {
				progressToken = request.Params.Meta.ProgressToken
			}

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

			sendProgress(0, 100, "🔍 Searching knowledge base...")
			searchResults, err := withEmbedderRetry(ctx, func() ([]keyvalembd.SearchResult, error) {
				return kv.SearchSemantic(question, topK)
			})
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

			var chunks []ragResult
			for _, sr := range searchResults {
				chunks = append(chunks, ragResult{
					Key:   sr.Key,
					Text:  sr.Text,
					Score: sr.Score,
				})
			}

			var chunkSummary []string
			for _, ch := range chunks {
				truncated := []rune(ch.Text)
				if len(truncated) > 120 {
					truncated = truncated[:120]
				}
				chunkSummary = append(chunkSummary, fmt.Sprintf(
					"- [%.4f] %s", ch.Score, string(truncated)+"..."))
			}

			style, _ := args["style"].(string)
			if style == "" {
				style = "creative"
			}

			messages, err := buildRAGPrompt(chunks, question, style)
			if err != nil {
				sendProgress(100, 100, "❌ Failed to build prompt")
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error building prompt: %v", err)), nil
			}

			const totalProgress = 100.0
			const progressStart = 30.0
			const progressEnd = 95.0

			var answerBuf strings.Builder
			tokenCount := 0
			progressFn := func(token string) {
				tokenCount++
				p := progressStart + (progressEnd-progressStart)*
					float64(tokenCount)/float64(tokenCount+50)
				if p > progressEnd {
					p = progressEnd
				}

				if isStreamMode && progressToken != nil {
					answerBuf.WriteString(token)
					sendProgress(p, totalProgress, fmt.Sprintf("answer_%s", token))
				} else {
					sendProgress(p, totalProgress, fmt.Sprintf("💬 Generating answer... (token %d)", tokenCount))
				}
			}

			answer, err := generateAnswerStreamWithOptions(messages, GenerateAnswerOptions{
				ProgressFn:     progressFn,
				StreamToStderr: streamAnswerToStderr,
			})
			if isStreamMode {
				if answerBuf.Len() > 0 {
					answer = answerBuf.String()
				}
			}
			if err != nil {
				sendProgress(100, 100, "❌ Answer generation failed")
				return mcp.NewToolResultText(fmt.Sprintf(
					"Error generating answer: %v", err)), nil
			}

			sendProgress(100, 100, "✅ Answer generated")

			result := fmt.Sprintf("Question: %s\n\n", question)
			result += fmt.Sprintf("Answer:\n%s\n\n", answer)
			result += fmt.Sprintf("Context (%d fragments):\n", len(chunks))
			result += strings.Join(chunkSummary, "\n")

			return mcp.NewToolResultText(result), nil
		},
	}
}
