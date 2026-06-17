# rag-mcp — Status

## Project Status: ✅ Active (v0.3.0)

## Milestones

| Date | Milestone | Status |
| ------------ | ----------- | -------- |
| 2026-05-02 | Initial build — MCP server with ingest + query + delete | ✅ Done |
| 2026-05-02 | Smoke test: tools/list | ✅ Done |
| 2026-05-02 | Fix race condition (sync.Mutex) | ✅ Done |
| 2026-05-02 | Switch to Ollama /api/chat (fix answer truncation) | ✅ Done |
| 2026-05-02 | Smoke test: ingest → query → LLM generation | ✅ Done |
| 2026-05-02 | Docs: CONTEXT.md, DESIGN.md, STATUS.md | ✅ Done |
| 2026-05-03 | rag-query progress notifications | ✅ Done |
| 2026-05-03 | file_path, directory, URL ingest — 6 tools total | ✅ Done |
| 2026-05-03 | Semantic chunking — sentence split, overlap, min 500 chars | ✅ Done |
| 2026-05-03 | rag-cli — standalone CLI with query/ingest/list/delete | ✅ Done |
| 2026-05-19 | Fix rag_query stderr streaming and cold-start embedder retries | ✅ Done |
| 2026-05-21 | Refactor: split tools.go into ingest.go, query.go, metadata.go | ✅ Done |
| 2026-06-17 | Remove dead code from chunker.go (#1) | ✅ Done |
| 2026-06-17 | Refactor: extract storeChunks, dedupe chunk storage loop across 3 tools | 🔄 In progress |

## Current State

- **6 tools**: `rag_ingest`, `rag_ingest_directory`, `rag_ingest_url`, `rag_query`, `rag_list`, `rag_delete` — all functional
- **CLI client**: `rag-cli` — standalone binary using MCP stdio client (Cobra framework)
  - `query` with real-time LLM token streaming to stderr
  - `ingest` with subcommands: `text`, `file`, `dir`, `url`
  - `list`, `delete`
  - Auto-discovers `rag-mcp` binary (PATH, same dir, GOPATH/bin)
- **LLM**: Uses Ollama `/api/chat` with `stream: true` for reliable NDJSON parsing
- **Storage**: keyvalembd (libSQL + vector embeddings)
- **Chunker**: Sentence-based semantic chunking with overlap (2 sentences) and target size 1200 chars (min 500, max 2000). Includes `generateDescription()` for auto doc descriptions.
- **Runtime safety**: `rag_query` no longer writes answer tokens to stderr by default, preventing blocked MCP clients; embedding/search operations retry briefly while keyvalembd initializes its embedder
- **Code structure**: Tools split into logical modules — `tools.go` (routing + list/delete), `ingest.go` (ingest tools using shared `storeChunks`), `query.go`, `metadata.go` (shared `storeChunks`, `storeMeta`, `deleteOldChunks`)
- **No known issues**

## Test Results

```txt
=== RUN   TestSmoke
✅ Embeddings ready (model: embeddinggemma:latest)
✅ keyvalembd ready
✅ Generated 1 chunks
✅ chunk stored
✅ Found 1 results (score=0.5827)
✅ LLM answer: "Cooksy is a recipe sharing platform..."
=== SMOKE TEST PASSED ===
--- PASS: TestSmoke (8.04s)
```

### rag-cli Tests

```bash
./rag-cli --help         # All commands displayed
./rag-cli list           # Listed 2 entries (rag/, sqlh/)
./rag-cli query "..."    # Connected to rag-mcp, streamed tokens, returned answer
```

## Next Steps

- [ ] Register rag-mcp as MCP server in Cline config
- [ ] Integrate with Cooksy knowledge base ingestion
- [ ] Add document deletion by specific chunk
- [ ] Add `rag-cli` shell completion scripts
- [ ] Add `rag-cli ingest batch` — bulk ingest from file list
- [ ] Add `--format` flag to `list` (json, tree)
