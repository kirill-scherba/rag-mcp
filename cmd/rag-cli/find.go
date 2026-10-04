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

func findCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "find <keyword>",
		Short: "Exact keyword search (SQL LIKE), no embeddings needed",
		Long: `Performs exact keyword search across ingested documents using SQL LIKE.
Complements 'search' (semantic). Works without Ollama.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			keyword := strings.Join(args, " ")
			toolArgs := map[string]any{
				"keyword": keyword,
			}
			if limit > 0 {
				toolArgs["limit"] = limit
			}

			result, err := globalClient.callTool(context.Background(), "rag_find", toolArgs)
			if err != nil {
				return err
			}
			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 0,
		"Maximum number of results (default: server-side limit)")

	return cmd
}
