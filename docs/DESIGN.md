# rag-mcp — Design

## Architecture

```
┌─────────────┐     JSON-RPC 2.0     ┌──────────────┐
│  MCP Client  │ ◄──── stdin/stdout ──► │  rag-mcp      │
│  (e.g., AI   │                       │  (Go binary)  │
│   assistant) │                       └──────┬───────┘
└─────────────┘                               │
         ▲                                    │
         │                                    ▼
         │                          ┌─────────────────┐
         │                          │   keyvalembd     │
         │                          │  (libSQL + vec)  │
         │                          └────────┬────────┘
         │                                   │
         │                                   ▼
         │                          ┌─────────────────┐
         │                          │    Ollama API    │
         │                          │  localhost:11434 │
         │                          └─────────────────┘
```

## Components

### 1. MCP Server Layer (`main.go`)
- Entry point, flag parsing, server lifecycle.
- Creates `keyvalembd` instance and registers tools.

### 2. Chunker (`chunker.go`)
- **Sentence-based splitting** — splits text at sentence boundaries (. ! ? … followed by whitespace/end).
- **Semantic chunking** — accumulates sentences until target size (~1200 chars), with a minimum of 500 chars and hard max of 2000 chars.
- **Overlap** — preserves the last 2 sentences from each chunk as overlap into the next chunk for context continuity.
- **Deduplication** — removes consecutive identical chunks that can occur with tiny documents.

### 3. Tools (`tools.go`)
- **`rag_ingest`**: Accepts `key` (document key) and either `text` (inline content) or `file_path` (path to file on disk). Splits into chunks, generates embeddings, stores in keyvalembd. Returns chunk count.
- **`rag_ingest_directory`**: Accepts `key_prefix`, `dir_path`, and optional `pattern` (default `*.md,*.txt`). Scans directory, reads each matching file, ingests with key `<key_prefix>/<filename>`.
- **`rag_ingest_url`**: Accepts `url` (required) and optional `key`. Fetches URL via HTTP GET, chunks and stores content. Auto-generates key from host+path if not provided.
- **`rag_query`**: Accepts `question` text. Performs semantic search on stored chunks, builds RAG prompt, calls LLM. Returns combined answer.
- **`rag_list`**: Lists document keys or chunks in the knowledge base.
- **`rag_delete`**: Accepts `key` (document key prefix). Lists all chunks with that prefix and deletes them from keyvalembd. Returns deleted count.

### 4. LLM Generation (`generate.go`)
- `buildRAGPrompt()` — formats context chunks + system instruction + user question into Ollama chat messages.
- `generateAnswerStreamWithOptions()` — sends request to Ollama `/api/chat` endpoint with `stream: true`, aggregates NDJSON token chunks, and optionally emits tokens to stderr only when explicitly enabled.

## Data Flow: Query

```
User query
  │
  ▼
semantic_search(key_prefix, query_embedding)
  │
  ▼
Top-k chunks with similarity scores
  │
  ▼
buildRAGPrompt(chunks, question)
  │
  ▼
generateAnswer(messages) → Ollama /api/chat
  │
  ▼
Answer text returned to user
```

## CLI Client: rag-cli

### Purpose

`rag-cli` is a standalone CLI binary that communicates with the `rag-mcp` MCP server via JSON-RPC 2.0 over stdin/stdout. It provides a familiar command-line interface to the RAG knowledge base without needing an MCP-compatible AI assistant.

### Architecture

```
┌──────────────┐    JSON-RPC 2.0    ┌──────────────┐
│   rag-cli     │ ◄── stdin/stdout ──► │   rag-mcp     │
│  (Cobra CLI)  │                     │  (MCP server) │
└──────────────┘                     └──────┬───────┘
                                            │
                                            ▼
                                  ┌─────────────────┐
                                  │   keyvalembd     │
                                  │  (libSQL + vec)  │
                                  └─────────────────┘
```

### Implementation

- **Location**: `cmd/rag-cli/`
- **Framework**: `spf13/cobra` for CLI structure
- **Client**: `mark3labs/mcp-go/client` — stdio MCP client to rag-mcp
- **Auto-discovery**: Locates `rag-mcp` binary in PATH, same directory, or `$GOPATH/bin`
- **Token streaming**: rag-mcp writes LLM tokens to stderr; rag-cli proxies this to user's stderr via `proxyStderrWithThinking()` for real-time streaming feedback with a "Thinking..." spinner until the first token arrives
- **Source files**:
  - `main.go` — root command, persistent flags (`--db`, `--model`)
  - `client.go` — MCP stdio client, auto-discovery, stderr proxy
  - `query.go` — `rag_query` wrapper with `--style` flag
  - `ingest.go` — subcommands: `text`, `file`, `dir`, `url`
  - `list.go` — `rag_list` wrapper
  - `delete.go` — `rag_delete` wrapper

### Commands

| Command | Description |
|---------|-------------|
| `rag-cli query <question>` | Semantic search + LLM answer (tokens streamed to stderr) |
| `rag-cli ingest text` | Ingest inline text (argument or stdin via `-`) |
| `rag-cli ingest file` | Ingest a file from disk |
| `rag-cli ingest dir` | Ingest all docs from a directory |
| `rag-cli ingest url` | Fetch URL and ingest content |
| `rag-cli list [key]` | List document keys or chunks |
| `rag-cli delete <key>` | Delete a document |

### Global Flags

| Flag | Description |
|------|-------------|
| `--db` | Override rag-mcp database path |
| `-m, --model` | Override LLM model |

### Build

```bash
go build -o rag-cli ./cmd/rag-cli/
```

Since rag-mcp must be available to spawn, either:
- Build both and keep them in the same directory
- Install rag-mcp to `$GOPATH/bin` or a PATH location

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Storage | keyvalembd (libSQL) | Embeddings in SQLite — no external vector DB needed |
| Embeddings | Ollama embeddinggemma | Local embedding gen, no API keys |
| LLM | Ollama phi4-mini | Local, small, free, moderate speed |
| Protocol | MCP JSON-RPC | Standard protocol for AI assistants |
| Streaming | Always `stream: true` | Ollama may return NDJSON even with `stream: false` |
| stderr token output | Disabled by default; enabled by `--stream-stderr` | Prevents blocked MCP clients when stderr is not drained; preserves rag-cli live token output |
| Embedder readiness | Retry `embedder is not ready` for 10 seconds | Handles keyvalembd/Ollama cold starts without hiding persistent failures |
| Chunk strategy | Sentence-based semantic | Splits by sentence boundaries, overlap preserves context |
| Chunk min size | 500 chars | Avoid trivial chunks, each chunk has enough semantic weight |
| Chunk target size | 1200 chars | Optimal for embedding models and LLM context windows |
| Chunk max size | 2000 chars | Hard safety limit to prevent oversized chunks |
| Chunk overlap | 2 sentences | Carried from previous chunk for context continuity |
| Max chunks per doc | 1000 | Safety limit |
| CLI framework | Cobra (spf13/cobra) | Standard Go CLI framework with subcommands |
| MCP client library | mark3labs/mcp-go/client | Official Go MCP client with stdio transport |
| Binary autodiscovery | PATH + same dir + GOPATH | Flexible deployment without configuration |

## Tool Specifications

### rag_ingest
```json
{
  "name": "rag_ingest",
  "inputSchema": {
    "type": "object",
    "properties": {
      "key":       { "type": "string", "description": "Document key/prefix (e.g. 'cooksy/architecture')" },
      "text":      { "type": "string", "description": "Document content to ingest (mutually exclusive with file_path)" },
      "file_path": { "type": "string", "description": "Path to file on disk to ingest (mutually exclusive with text)" }
    },
    "required": ["key"]
  }
}
```

### rag_ingest_directory
```json
{
  "name": "rag_ingest_directory",
  "inputSchema": {
    "type": "object",
    "properties": {
      "key_prefix": { "type": "string", "description": "Prefix for document keys (e.g. rag/docs/cooksy)" },
      "dir_path":   { "type": "string", "description": "Path to directory containing documents" },
      "pattern":    { "type": "string", "description": "Glob pattern (default: *.md,*.txt)" }
    },
    "required": ["key_prefix", "dir_path"]
  }
}
```

### rag_ingest_url
```json
{
  "name": "rag_ingest_url",
  "inputSchema": {
    "type": "object",
    "properties": {
      "key": { "type": "string", "description": "Document key (auto-generated from URL if empty)" },
      "url": { "type": "string", "description": "URL to fetch and ingest" }
    },
    "required": ["url"]
  }
}
```

### rag_query
```json
{
  "name": "rag_query",
  "inputSchema": {
    "type": "object",
    "properties": {
      "question": { "type": "string", "description": "Natural language query" },
      "top_k":    { "type": "number", "description": "Max chunks to retrieve (default 5, max 20)", "default": 5 }
    },
    "required": ["question"]
  }
}
```

### rag_list
```json
{
  "name": "rag_list",
  "inputSchema": {
    "type": "object",
    "properties": {
      "key": { "type": "string", "description": "Optional key prefix to list (lists all top-level keys if omitted)" }
    }
  }
}
```

### rag_delete
```json
{
  "name": "rag_delete",
  "inputSchema": {
    "type": "object",
    "properties": {
      "key": { "type": "string", "description": "Document key/prefix to delete" }
    },
    "required": ["key"]
  }
}
