// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// rag-mcp — MCP server for RAG (Retrieval-Augmented Generation) knowledge base.
//
// This server provides a complete RAG pipeline as MCP tools:
//   - rag_ingest: Ingest documents (chunk, embed, store)
//   - rag_query:  Answer questions using semantic search + LLM
//   - rag_delete: Remove documents from the knowledge base
//
// Architecture:
//   - Uses keyvalembd (libSQL + Ollama embeddings) for storage
//   - Chunks documents by paragraphs with min chunk size (100 chars)
//   - Generates answers via Ollama LLM (qwen2.5-7B by default)
//   - Implements MCP (Model Context Protocol) via JSON-RPC 2.0 over stdin/stdout
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/kirill-scherba/keyvalembd"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Command line flags
	dbPath := flag.String("db", "",
		"Path to the database (default: ~/.config/rag-mcp/rag.db)")
	model := flag.String("model", "",
		"LLM model for answer generation (overrides LLM_MODEL env, default: gemma3:4b)")
	showHelp := flag.Bool("h", false, "Show help")
	flag.Parse()

	// Apply model override
	if *model != "" {
		ollamaModelOverride = *model
	}

	if *showHelp {
		fmt.Fprintf(os.Stderr, "Usage: rag-mcp [options]\n\n")
		fmt.Fprintf(os.Stderr, "MCP server for RAG (Retrieval-Augmented Generation) knowledge base.\n")
		fmt.Fprintf(os.Stderr, "Communicates via JSON-RPC 2.0 over stdin/stdout.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  OLLAMA_BASE_URL     Ollama API URL (default: http://localhost:11434)\n")
		fmt.Fprintf(os.Stderr, "  EMBEDDING_MODEL     Embedding model (default: embeddinggemma:latest)\n")
		fmt.Fprintf(os.Stderr, "  LLM_MODEL           LLM model for answer generation (default: gemma3:4b)\n")
		fmt.Fprintf(os.Stderr, "\nModel priority: --model flag > LLM_MODEL env > default (gemma3:4b)\n")
		os.Exit(0)
	}

	// Default db path
	if *dbPath == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			log.Fatalf("Could not determine config directory: %v", err)
		}
		*dbPath = filepath.Join(configDir, "rag-mcp", "rag.db")
	}

	// Ensure directory exists
	dir := filepath.Dir(*dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatalf("Could not create database directory %s: %v", dir, err)
	}

	// Initialize keyvalembd (libSQL + Ollama embeddings)
	kv, err := keyvalembd.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize keyvalembd: %v", err)
	}
	defer kv.Close()

	log.Printf("🚀 Starting rag-mcp server")
	log.Printf("   DB path: %s", *dbPath)

	// Create MCP server
	s := server.NewMCPServer(
		"rag-mcp",
		"0.2.0",
		server.WithInstructions(`RAG MCP — Retrieval-Augmented Generation knowledge base.

Ingest documents, then ask questions. The system will:
1. Split documents into chunks
2. Generate embeddings for each chunk
3. Store chunks for semantic search
4. On query: find relevant chunks + generate answer via LLM

Available tools:
- rag_ingest:           Ingest a document (by text or file_path)
- rag_ingest_directory: Ingest all documents from a directory
- rag_ingest_url:       Fetch a URL and ingest its content
- rag_query:            Ask a question (semantic search + LLM answer)
- rag_list:             List stored documents
- rag_delete:           Delete a document and all its chunks`),
	)

	// Register all tools
	s.AddTools(tools(s, kv)...)

	log.Printf("✅ Registered 6 tools")

	// Start the server over stdin/stdout (JSON-RPC 2.0)
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}