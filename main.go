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
//   - Generates answers via Ollama LLM (deepseek-v4.1-flash:cloud by default)
//   - Implements MCP (Model Context Protocol) via JSON-RPC 2.0 over stdin/stdout
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/kirill-scherba/rag-mcp/rag"
	"github.com/mark3labs/mcp-go/server"
)

// ClientMode controls how rag_query streams answer tokens.
type ClientMode int

const (
	// ClientModeAuto detects the client automatically from ClientInfo.
	ClientModeAuto ClientMode = iota
	// ClientModeBatch returns the full answer in CallToolResult (default for CLI clients).
	ClientModeBatch
	// ClientModeStream sends answer tokens via progress notifications (default for Cline/IDE clients).
	ClientModeStream
)

// clientMode holds the current client mode (set at startup, readable globally).
var clientMode ClientMode

// mu serializes tool handlers so the single rag.Store is accessed one call at a
// time (MCP requests may otherwise interleave over the stdio stream).
var mu sync.Mutex

// streamAnswerToStderr enables legacy token streaming through stderr.
// It is off by default because MCP clients may not drain stderr pipes.
var streamAnswerToStderr bool

func main() {
	// Command line flags
	dbPath := flag.String("db", "",
		"Path to the database (default: ~/.config/rag-mcp/rag.db)")
	model := flag.String("model", "",
		"LLM model for answer generation (overrides LLM_MODEL env, default: deepseek-v4.1-flash:cloud)")
	mode := flag.String("client-mode", "auto",
		"Client answer delivery mode: auto (detect from client name), batch (full answer in result), stream (tokens via progress)")
	streamStderr := flag.Bool("stream-stderr", false,
		"Stream answer tokens to stderr (intended for rag-cli, which drains stderr)")
	showHelp := flag.Bool("h", false, "Show help")
	flag.Parse()

	// Apply mode override
	switch *mode {
	case "auto":
		clientMode = ClientModeAuto
	case "batch":
		clientMode = ClientModeBatch
	case "stream":
		clientMode = ClientModeStream
	default:
		fmt.Fprintf(os.Stderr, "Invalid --client-mode value: %q (must be auto, batch, or stream)\n", *mode)
		os.Exit(1)
	}

	// Apply model override
	if *model != "" {
		ollamaModelOverride = *model
	}
	streamAnswerToStderr = *streamStderr

	if *showHelp {
		fmt.Fprintf(os.Stderr, "Usage: rag-mcp [options]\n\n")
		fmt.Fprintf(os.Stderr, "MCP server for RAG (Retrieval-Augmented Generation) knowledge base.\n")
		fmt.Fprintf(os.Stderr, "Communicates via JSON-RPC 2.0 over stdin/stdout.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  OLLAMA_BASE_URL     Ollama API URL (default: http://localhost:11434)\n")
		fmt.Fprintf(os.Stderr, "  EMBEDDING_MODEL     Embedding model (default: embeddinggemma:latest)\n")
		fmt.Fprintf(os.Stderr, "  LLM_MODEL           LLM model for answer generation (default: deepseek-v4.1-flash:cloud)\n")
		fmt.Fprintf(os.Stderr, "  EXTRACTOR_CMD       Command to turn an ingested file into text (e.g. 'pdftotext {file} -');\n")
		fmt.Fprintf(os.Stderr, "                      when unset, files are read as text\n")
		fmt.Fprintf(os.Stderr, "  EXTRACTOR_TIMEOUT   Per-file extractor timeout (default: 10m)\n")
		fmt.Fprintf(os.Stderr, "\nModel priority: --model flag > LLM_MODEL env > default (deepseek-v4.1-flash:cloud)\n")
		fmt.Fprintf(os.Stderr, "\nClient mode priority: --client-mode flag > auto-detect > default (auto)\n")
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

	// Initialize the RAG engine (keyvalembd storage + keyword index)
	store, err := rag.Open(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize rag store: %v", err)
	}
	defer store.Close()

	log.Printf("🚀 Starting rag-mcp server")
	log.Printf("   DB path: %s", *dbPath)
	log.Printf("   Client mode: %s", *mode)
	log.Printf("   Stream stderr: %v", streamAnswerToStderr)

	// Create MCP server
	s := server.NewMCPServer(
		"rag-mcp",
		"0.3.0",
		server.WithInstructions(`RAG MCP — Document knowledge base with semantic search.

Use rag_query (default style=creative) for narrative questions about
events, characters, places, stories. Use style=strict only when exact
citations are required (code, dates, specs).

At session start, call rag_list to see what documents are available.

Available tools:
- rag_ingest:           Ingest a document (by text or file_path)
- rag_ingest_directory: Ingest all documents from a directory
- rag_ingest_url:       Fetch a URL and ingest its content
- rag_search:           Search chunks by semantic similarity (returns chunks + scores)
- rag_find:             Exact keyword search (SQL LIKE; no embeddings needed)
- rag_query:            Ask a question (semantic search + LLM answer)
- rag_list:             List stored documents
- rag_delete:           Delete a document and all its chunks`),
	)

	// Register all tools
	s.AddTools(tools(s, store)...)

	log.Printf("✅ Registered 8 tools")

	// Start the server over stdin/stdout (JSON-RPC 2.0)
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
