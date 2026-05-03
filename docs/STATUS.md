# rag-mcp — Status

## Project Status: ✅ Active (v0.3.0)

## Milestones

| Date | Milestone | Status |
|------|-----------|--------|
| 2026-05-02 | Initial build — MCP server with ingest + query + delete | ✅ Done |
| 2026-05-02 | Smoke test: tools/list | ✅ Done |
| 2026-05-02 | Fix race condition (sync.Mutex) | ✅ Done |
| 2026-05-02 | Switch to Ollama /api/chat (fix answer truncation) | ✅ Done |
| 2026-05-02 | Smoke test: ingest → query → LLM generation | ✅ Done |
| 2026-05-02 | Docs: CONTEXT.md, DESIGN.md, STATUS.md | ✅ Done |
| 2026-05-03 | rag-query progress notifications | ✅ Done |
| 2026-05-03 | file_path, directory, URL ingest — 6 tools total | ✅ Done |
| 2026-05-03 | Semantic chunking — sentence split, overlap, min 500 chars | ✅ Done |

## Current State

- **6 tools**: `rag_ingest`, `rag_ingest_directory`, `rag_ingest_url`, `rag_query`, `rag_list`, `rag_delete` — all functional
- **LLM**: Uses Ollama `/api/chat` with `stream: true` for reliable NDJSON parsing
- **Storage**: keyvalembd (libSQL + vector embeddings)
- **Chunker**: Sentence-based semantic chunking with overlap (2 sentences) and target size 1200 chars (min 500, max 2000)
- **No known issues**

## Test Results

```
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

## Next Steps

- [ ] Register rag-mcp as MCP server in Cline config
- [ ] Integrate with Cooksy knowledge base ingestion
- [ ] Add `rag_list` tool to list all documents
- [ ] Add document deletion by specific chunk