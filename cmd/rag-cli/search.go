// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func searchCmd() *cobra.Command {
	var topK int
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Semantic search without LLM generation",
		Long: `Performs semantic search across ingested documents and returns
matching chunks with relevance scores. Does not invoke the LLM —
useful for debugging embedding quality or for quick lookups.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			ctx := context.Background()

			toolArgs := map[string]any{
				"query": query,
			}
			if topK > 0 {
				toolArgs["top_k"] = topK
			}

			result, err := globalClient.callTool(ctx, "rag_search", toolArgs)
			if err != nil {
				return err
			}
			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().IntVarP(&topK, "top-k", "k", 0,
		"Number of results to return (default: server-side limit)")

	return cmd
}
