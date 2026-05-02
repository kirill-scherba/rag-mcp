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
- **`rag_ingest`**: Accepts `id` (document key) and `text` (content). Splits into chunks, generates embeddings, stores in keyvalembd. Returns chunk count.
- **`rag_query`**: Accepts `query` text. Performs semantic search on stored chunks, builds RAG prompt, calls LLM. Returns combined answer.
- **`rag_delete`**: Accepts `id` (document key prefix). Lists all chunks with that prefix and deletes them from keyvalembd. Returns deleted count.

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
      "id":  { "type": "string", "description": "Document key/prefix (e.g. 'cooksy/architecture')" },
      "text": { "type": "string", "description": "Document content to ingest" }
    },
    "required": ["id", "text"]
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
      "query": { "type": "string", "description": "Natural language query" },
      "limit": { "type": "integer", "description": "Max chunks to retrieve (default 5)", "default": 5 }
    },
    "required": ["query"]
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
      "id": { "type": "string", "description": "Document key/prefix to delete" }
    },
    "required": ["id"]
  }
}