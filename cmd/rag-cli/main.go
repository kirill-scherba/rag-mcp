// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// rag-cli is a standalone CLI client for rag-mcp.
// It communicates with the rag-mcp MCP server via JSON-RPC over stdin/stdout.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	dbPath        string
	modelOverride string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "rag-cli",
		Short: "Standalone CLI for rag-mcp RAG knowledge base",
		Long: `rag-cli — a command-line interface to the rag-mcp MCP server.

Connects to rag-mcp via stdin/stdout JSON-RPC, providing ingest,
query, list, and delete operations without an AI assistant.`,
		PersistentPreRunE: persistentPreRun,
	}

	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "",
		"Override rag-mcp database path")
	rootCmd.PersistentFlags().StringVarP(&modelOverride, "model", "m", "",
		"Override LLM model for answer generation")

	rootCmd.AddCommand(
		queryCmd(),
		ingestCmd(),
		listCmd(),
		deleteCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// persistentPreRun discovers rag-mcp and establishes the MCP client.
func persistentPreRun(cmd *cobra.Command, args []string) error {
	if err := initClient(); err != nil {
		return fmt.Errorf("failed to initialize MCP client: %w", err)
	}
	return nil
}

// getExecutablePath returns the directory where this binary is located.
func getExecutablePath() string {
	ex, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(ex)
}

// goPathBin returns $GOPATH/bin or default GOPATH/bin.
func goPathBin() string {
	if g := os.Getenv("GOPATH"); g != "" {
		return filepath.Join(g, "bin")
	}
	// Default GOPATH
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "go", "bin")
}

// detectOS returns the GOOS-style OS name for binary suffixes.
func detectOS() string {
	goos := runtime.GOOS
	if goos == "" {
		return "linux"
	}
	return goos
}
