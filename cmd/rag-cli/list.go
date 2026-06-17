// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func listCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [key_prefix]",
		Short: "List documents in the knowledge base",
		Long: `Without arguments, lists all documents with descriptions.
With a key prefix, shows documents under that prefix.
With a specific document key, shows its metadata and chunks.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			toolArgs := map[string]any{}
			if len(args) > 0 {
				toolArgs["key"] = args[0]
			}

			result, err := globalClient.callTool(ctx, "rag_list", toolArgs)
			if err != nil {
				return err
			}
			fmt.Println(result)
			return nil
		},
	}

	return cmd
}
