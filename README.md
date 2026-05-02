# rag-mcp

**RAG MCP Server** — Retrieval-Augmented Generation knowledge base as an MCP (Model Context Protocol) server. Ingest documents, store them as vector embeddings, and answer questions by combining semantic search with LLM-generated answers.

## Features

- **Document Ingestion** (`rag_ingest`): Split text into chunks, generate embeddings, store in libSQL.
- **Semantic Search & QA** (`rag_query`): Find relevant chunks by meaning and generate answers via Ollama LLM.
- **List Documents** (`rag_list`): List document keys or chunks in the knowledge base.
- **Document Deletion** (`rag_delete`): Remove documents from the knowledge base.
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
  - `gemma3:4b` (or custom via `LLM_MODEL` env var)

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
  -db string   Path to the database (default: ~/.config/rag-mcp/rag.db)
  -h           Show help

Environment variables:
  OLLAMA_BASE_URL     Ollama API URL (default: http://localhost:11434)
  EMBEDDING_MODEL     Embedding model (default: embeddinggemma:latest)
  LLM_MODEL           LLM model for answer generation (default: gemma3:4b)
```

### MCP Tools

| Tool | Description |
|------|-------------|
| `rag_ingest` | Ingest a document — chunk, embed, and store text for semantic search |
| `rag_query` | Ask a question — semantic search + LLM answer generation |
| `rag_list` | List document keys or chunks in the knowledge base |
| `rag_delete` | Delete a document and all its chunks from the knowledge base |

## Example

**Ingest a document:**
```json
{
  "key": "cooksy/architecture",
  "text": "Cooksy is a recipe sharing platform..."
}
```

**Ask a question:**
```json
{
  "question": "What is Cooksy?"
}
```

**List documents:**
```json
{
  "key": "rag/docs"
}
```

**Delete a document:**
```json
{
  "key": "cooksy/architecture"
}
```

## How It Works

1. **Ingestion**: Text is split into paragraphs, combined into chunks (min 100 chars each), each chunk gets an Ollama embedding, and is stored in a libSQL database via `keyvalembd`.
2. **Query**: The question is embedded, semantically searched against stored chunks, top-k matches are retrieved, and a RAG prompt is built and sent to Ollama for answer generation.
3. **Deletion**: All chunks under a given key prefix are listed and removed.

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