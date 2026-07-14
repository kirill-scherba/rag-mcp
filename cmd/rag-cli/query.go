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

func queryCmd() *cobra.Command {
	var style string
	cmd := &cobra.Command{
		Use:   "query <question>",
		Short: "Ask a question using the RAG knowledge base",
		Long: `Performs semantic search across ingested documents and
generates an answer using the LLM. Tokens are streamed to stderr
in real time.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			question := strings.Join(args, " ")
			ctx := context.Background()

			toolArgs := map[string]any{
				"question": question,
			}
			if style != "" {
				toolArgs["style"] = style
			}

			result, err := globalClient.callTool(ctx, "rag_query", toolArgs)
			if err != nil {
				return err
			}
			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().StringVarP(&style, "style", "s", "",
		"Answer style: creative (default, free-form) or strict (exact copy-paste)")

	return cmd
}
