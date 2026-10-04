# rag-mcp — Context

## Overview

rag-mcp is an MCP (Model Context Protocol) server that provides Retrieval-Augmented Generation (RAG) capabilities. It allows ingesting documents, storing them as vector embeddings, and answering questions by combining semantic search with LLM-generated answers.

## Purpose

- Provide a pluggable RAG knowledge base accessible via MCP tools.
- Enable AI assistants to store and retrieve contextual knowledge from documents.
- Serve as the knowledge backend for the Cooksy project and other applications.

## Key Features

- **Document Ingestion** (`rag_ingest`): Split text into chunks, generate embeddings, store in libSQL.
- **Semantic Search & QA** (`rag_query`): Find relevant chunks and answer questions via LLM.
- **Raw Semantic Search** (`rag_search`): Search for relevant chunks and return them with similarity scores, without LLM generation.
- **Exact Keyword Search** (`rag_find`): SQL LIKE search over chunk text and document descriptions; needs no embeddings and works with Ollama down (complements semantic search).
- **List Documents** (`rag_list`): List document keys or chunks in the knowledge base; detailed view shows chunk text previews.
- **Document Deletion** (`rag_delete`): Remove documents and all their chunks.
- **MCP Protocol**: JSON-RPC 2.0 over stdin/stdout — works with any MCP client.

## Integration

- Uses [keyvalembd](https://github.com/kirill-scherba/keyvalembd) for libSQL-backed key-value store with vector embeddings.
- Uses [anytext/totext](https://github.com/kirill-scherba/anytext) to turn any ingested file (text, DOCX, HTML, PDF, images) into text.
- Uses Ollama for both embeddings (`embeddinggemma:latest`) and answer generation (`deepseek-v4.1-flash:cloud`).

### Runtime dependencies and provider note

- **LLM (answer generation)** is used only by `rag_query`. `rag_search`,
  `rag_find`, `rag_list`, `rag_ingest*` and `rag_delete` do not call an LLM, so
  a client that only searches or ingests needs no chat model.
- **Embeddings** (Ollama `embeddinggemma:latest`) are required by ingest and by
  semantic search. This is intentional: Ollama is a runtime dependency of the
  RAG — and therefore of any GUI built on top of it. Embeddings use a fixed,
  good local model.
- **Model name is provider-specific.** `deepseek-v4.1-flash:cloud` is an Ollama
  cloud-model identifier. A future GUI may use any provider; the model must be
  selectable via `LLM_MODEL` / `--model`, never hard-coded at call sites.
- **Never write to the production database from tests.** `--db` selects the
  database; every experiment must point at a copy or a scratch path.
- Implements MCP via [mcp-go](https://github.com/mark3labs/mcp-go) SDK.

## Recent Fixes

- 2026-10-04: **keyvalembd upgraded to v0.6.1** (pure-Go `modernc.org/sqlite` + in-process `vecindex`, DiskANN/libSQL removed). The existing database keeps its legacy `embedding` BLOB column and keyvalembd reads it transparently; verified by running the same semantic query before and after — **identical results and scores**. Also bumped `anytext` to v0.2.3.
- 2026-10-04: **answer model fixed** to `deepseek-v4.1-flash:cloud` (the previous `deepseek-v4-flash:cloud` was retired and Ollama returned HTTP 410). The model is provider-specific and overridable with `LLM_MODEL` or `--model`; it is the single knob — see the provider note below.
- 2026-10-04: `rag_ingest` and `rag_ingest_directory` convert `file_path` through the [anytext/totext](https://github.com/kirill-scherba/anytext) library instead of `os.ReadFile`, so any supported document can be ingested directly: text, Markdown, HTML, CSV/TSV, DOCX, PDF (native text plus OCR for scanned pages) and images (OCR). Pages are joined with a blank line before chunking.
- 2026-05-19: `rag_query` now keeps stderr token streaming disabled by default to avoid blocking MCP clients that do not drain stderr pipes. `rag-cli` can still enable legacy stderr token streaming with `--stream-stderr`.
- 2026-05-19: Embedding writes and semantic search retry `embedder is not ready` during keyvalembd/Ollama cold start before returning an error.

## Author

Kirill Scherba
