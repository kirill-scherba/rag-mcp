// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func ingestCmd() *cobra.Command {
	ingestCmd := &cobra.Command{
		Use:   "ingest",
		Short: "Ingest documents into the RAG knowledge base",
		Long:  `Ingest text, files, directories, or URLs as documents.`,
	}

	ingestCmd.AddCommand(
		ingestTextCmd(),
		ingestFileCmd(),
		ingestDirCmd(),
		ingestUrlCmd(),
	)

	return ingestCmd
}

func ingestTextCmd() *cobra.Command {
	var key, description string
	cmd := &cobra.Command{
		Use:   "text <text> | text -",
		Short: "Ingest inline text (or read from stdin via -)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var text string
			if args[0] == "-" {
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("reading stdin: %w", err)
				}
				text = string(data)
			} else {
				text = args[0]
			}

			if key == "" {
				return fmt.Errorf("--key is required")
			}

			toolArgs := map[string]any{
				"key":  key,
				"text": text,
			}
			if description != "" {
				toolArgs["description"] = description
			}

			result, err := globalClient.callTool(context.Background(), "rag_ingest", toolArgs)
			if err != nil {
				return err
			}
			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().StringVarP(&key, "key", "k", "", "Document key (e.g. rag/docs/cooksy)")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Optional description (auto-generated if empty)")
	_ = cmd.MarkFlagRequired("key")

	return cmd
}

func ingestFileCmd() *cobra.Command {
	var key, description string
	cmd := &cobra.Command{
		Use:   "file <file_path>",
		Short: "Ingest a file from disk",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			if key == "" {
				return fmt.Errorf("--key is required")
			}

			toolArgs := map[string]any{
				"key":       key,
				"file_path": filePath,
			}
			if description != "" {
				toolArgs["description"] = description
			}

			result, err := globalClient.callTool(context.Background(), "rag_ingest", toolArgs)
			if err != nil {
				return err
			}
			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().StringVarP(&key, "key", "k", "", "Document key")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Optional description")
	_ = cmd.MarkFlagRequired("key")

	return cmd
}

func ingestDirCmd() *cobra.Command {
	var keyPrefix, pattern string
	cmd := &cobra.Command{
		Use:   "dir <dir_path>",
		Short: "Ingest all documents from a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dirPath := args[0]
			if keyPrefix == "" {
				return fmt.Errorf("--key-prefix is required")
			}

			toolArgs := map[string]any{
				"key_prefix": keyPrefix,
				"dir_path":   dirPath,
			}
			if pattern != "" {
				toolArgs["pattern"] = pattern
			}

			result, err := globalClient.callTool(context.Background(), "rag_ingest_directory", toolArgs)
			if err != nil {
				return err
			}
			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().StringVar(&keyPrefix, "key-prefix", "", "Prefix for document keys")
	cmd.Flags().StringVarP(&pattern, "pattern", "p", "*.md,*.txt", "Glob pattern for files")
	_ = cmd.MarkFlagRequired("key-prefix")

	return cmd
}

func ingestUrlCmd() *cobra.Command {
	var key string
	cmd := &cobra.Command{
		Use:   "url <url>",
		Short: "Fetch a URL and ingest its content",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			urlStr := args[0]
			toolArgs := map[string]any{
				"url": urlStr,
			}
			if key != "" {
				toolArgs["key"] = key
			}

			result, err := globalClient.callTool(context.Background(), "rag_ingest_url", toolArgs)
			if err != nil {
				return err
			}
			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().StringVarP(&key, "key", "k", "", "Document key (auto-generated from URL if empty)")

	return cmd
}
