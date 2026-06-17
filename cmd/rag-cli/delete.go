// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func deleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <key>",
		Short: "Delete a document and all its chunks",
		Long:  `Removes a document and every chunk under its key from the knowledge base.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			ctx := context.Background()

			result, err := globalClient.callTool(ctx, "rag_delete", map[string]any{
				"key": key,
			})
			if err != nil {
				return err
			}
			fmt.Println(result)
			return nil
		},
	}

	return cmd
}
