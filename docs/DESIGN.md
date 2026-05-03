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
- `splitIntoParagraphs()` — splits text by blank lines.
- `splitLongParagraph()` — splits paragraphs > 200 chars by sentences.
- `makeChunks()` — combines paragraphs into chunks of min 100 chars.

### 3. Tools (`tools.go`)
- **`rag_ingest`**: Accepts `key` (document key) and either `text` (inline content) or `file_path` (path to file on disk). Splits into chunks, generates embeddings, stores in keyvalembd. Returns chunk count.
- **`rag_ingest_directory`**: Accepts `key_prefix`, `dir_path`, and optional `pattern` (default `*.md,*.txt`). Scans directory, reads each matching file, ingests with key `<key_prefix>/<filename>`.
- **`rag_ingest_url`**: Accepts `url` (required) and optional `key`. Fetches URL via HTTP GET, chunks and stores content. Auto-generates key from host+path if not provided.
- **`rag_query`**: Accepts `question` text. Performs semantic search on stored chunks, builds RAG prompt, calls LLM. Returns combined answer.
- **`rag_list`**: Lists document keys or chunks in the knowledge base.
- **`rag_delete`**: Accepts `key` (document key prefix). Lists all chunks with that prefix and deletes them from keyvalembd. Returns deleted count.

### 4. LLM Generation (`generate.go`)
- `buildRAGPrompt()` — formats context chunks + system instruction + user question into Ollama chat messages.
- `generateAnswer()` — sends request to Ollama `/api/chat` endpoint with `stream: true`. Handles both streaming (NDJSON) and non-streaming responses. Aggregates content parts.

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

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Storage | keyvalembd (libSQL) | Embeddings in SQLite — no external vector DB needed |
| Embeddings | Ollama embeddinggemma | Local embedding gen, no API keys |
| LLM | Ollama qwen2.5-7B | Local, fast, good quality |
| Protocol | MCP JSON-RPC | Standard protocol for AI assistants |
| Streaming | Always `stream: true` | Ollama may return NDJSON even with `stream: false` |
| Chunk min size | 100 chars | Avoid trivial chunks |
| Max chunks per doc | 1000 | Safety limit |

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