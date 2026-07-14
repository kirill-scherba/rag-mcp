# rag-mcp

**RAG MCP Server** — Retrieval-Augmented Generation knowledge base as an MCP (Model Context Protocol) server. Ingest documents, store them as vector embeddings, and answer questions by combining semantic search with LLM-generated answers.

## Features

- **Document Ingestion** (`rag_ingest`): Split text into semantic chunks, generate embeddings, store in libSQL. Supports inline text or file path. Auto-generates description from text.
- **Batch Directory Ingestion** (`rag_ingest_directory`): Ingest all `.md`/`.txt` files from a directory in one call.
- **URL Ingestion** (`rag_ingest_url`): Fetch a web page and ingest its content.
- **Raw Semantic Search** (`rag_search`): Search the knowledge base for relevant chunks and return them with similarity scores. No LLM generation — useful for debugging embeddings and for tools that need raw results.
- **Semantic Search & QA** (`rag_query`): Find relevant chunks by meaning and generate answers via Ollama LLM.
- **Smart Document Listing** (`rag_list`): Recursive listing with descriptions, chunk counts, and stored dates. Shows document details when given a specific key, including the first 100 characters of each chunk's text.
- **Document Deletion** (`rag_delete`): Remove documents and all their chunks from the knowledge base.
- **Duplicate Prevention**: Re-ingesting a document automatically deletes old chunks before storing new ones.
- **MCP Protocol**: JSON-RPC 2.0 over stdin/stdout — works with any MCP client (AI assistants, tools, etc.).

## Architecture

```
┌─────────────┐     JSON-RPC 2.0     ┌──────────────┐
│  MCP Client  │ ◄──── stdin/stdout ──► │  rag-mcp      │
│  (e.g., AI   │                       │  (Go binary)  │
│   assistant) │                       └──────┬───────┘
└─────────────┘                               │
                                              ▼
                                     ┌─────────────────┐
                                     │   keyvalembd     │
                                     │  (libSQL + vec)  │
                                     └────────┬────────┘
                                              │
                                              ▼
                                     ┌─────────────────┐
                                     │    Ollama API    │
                                     │  localhost:11434 │
                                     └─────────────────┘
```

## Prerequisites

- [Go](https://go.dev/dl/) 1.26+
- [Ollama](https://ollama.com/) running locally with:
  - `embeddinggemma:latest` (or custom via `EMBEDDING_MODEL` env var)
  - `phi4-mini` (or custom via `LLM_MODEL` env var)

## Installation

```bash
git clone git@github.com:kirill-scherba/rag-mcp.git
cd rag-mcp
go build -o rag-mcp .
```

## Usage

### As a standalone MCP server

```bash
./rag-mcp
```

The server communicates via JSON-RPC 2.0 over stdin/stdout. Connect it as an MCP tool in your AI assistant's configuration.

### Options

```
Usage: rag-mcp [options]

MCP server for RAG (Retrieval-Augmented Generation) knowledge base.
Communicates via JSON-RPC 2.0 over stdin/stdout.

Options:
  -db string           Path to the database (default: ~/.config/rag-mcp/rag.db)
  -model string        LLM model for answer generation (overrides LLM_MODEL env)
  -client-mode string  Answer delivery: auto, batch, or stream (default: auto)
  -stream-stderr       Stream answer tokens to stderr (for rag-cli)
  -h                   Show help

Environment variables:
  OLLAMA_BASE_URL     Ollama API URL (default: http://localhost:11434)
  EMBEDDING_MODEL     Embedding model (default: embeddinggemma:latest)
  LLM_MODEL           LLM model for answer generation (default: phi4-mini)

Model priority: --model flag > LLM_MODEL env > default (phi4-mini)
```

### MCP Tools

| Tool | Description |
|------|-------------|
| `rag_ingest` | Ingest a document — chunk, embed, and store text for semantic search. Optional `description` (auto-generated if empty). |
| `rag_ingest_directory` | Ingest all files from a directory. Auto-generates document keys from filenames. |
| `rag_ingest_url` | Fetch a URL and ingest its content. Auto-generates key from URL if not provided. |
| `rag_search` | Search chunks by semantic similarity. Returns matching chunks with scores and text previews. |
| `rag_query` | Ask a question — semantic search + LLM answer generation |
| `rag_list` | List documents recursively with descriptions and chunk counts. Show details for a specific key, including chunk text previews. |
| `rag_delete` | Delete a document and all its chunks from the knowledge base |

## Example

**Ingest a document:**
```json
{
  "key": "cooksy/architecture",
  "text": "Cooksy is a recipe sharing platform...",
  "description": "Cooksy platform overview (auto-generated if omitted)"
}
```

**Ingest a directory:**
```json
{
  "key_prefix": "rag/docs/cooksy",
  "dir_path": "/path/to/docs",
  "pattern": "*.md,*.txt"
}
```

**Ingest a URL:**
```json
{
  "url": "https://example.com/docs",
  "key": "rag/web/example/docs"
}
```

**Search chunks:**
```json
{
  "query": "Cooksy features",
  "top_k": 3
}
```

**Ask a question:**
```json
{
  "question": "What is Cooksy?"
}
```

**List all documents:**
```json
{}
```

**List documents under a prefix:**
```json
{
  "key": "rag/docs/cooksy"
}
```

**Show document details:**
```json
{
  "key": "rag/docs/cooksy/architecture"
}
```

**Delete a document:**
```json
{
  "key": "cooksy/architecture"
}
```

## How It Works

1. **Ingestion**: Text is split into sentences and combined into semantically meaningful chunks (target ~1200 chars, min 500, max 2000). Each chunk gets an Ollama embedding and is stored in a libSQL database via `keyvalembd`. A metadata record (`doc_key/meta`) stores description, chunk count, and timestamp.
2. **Re-ingestion**: If a document key already exists, old chunks and metadata are deleted before new ones are stored — preventing duplicates.
3. **Query**: The question is embedded, semantically searched against stored chunks, top-k matches are retrieved, and a RAG prompt is built and sent to Ollama for answer generation.
4. **Deletion**: All chunks and metadata under a given document key are listed and removed.

## Document Key Structure

```
doc_key/meta          — metadata (description, num_chunks, stored)
doc_key/chunk/0000    — chunk with embedding
doc_key/chunk/0001    — next chunk
...
```

## Dependencies

- [keyvalembd](https://github.com/kirill-scherba/keyvalembd) — libSQL-backed key-value store with vector embeddings
- [mcp-go](https://github.com/mark3labs/mcp-go) — MCP SDK for Go
- [Ollama](https://ollama.com/) — local embedding and LLM inference

## Documentation

- [CONTEXT.md](docs/CONTEXT.md) — Project context and overview
- [DESIGN.md](docs/DESIGN.md) — Architectural design and decisions
- [STATUS.md](docs/STATUS.md) — Current status and roadmap

## License

BSD-style license. Use of this source code is governed by a BSD-style license.

## Author

Kirill Scherba