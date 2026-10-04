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
- **List Documents** (`rag_list`): List document keys or chunks in the knowledge base; detailed view shows chunk text previews.
- **Document Deletion** (`rag_delete`): Remove documents and all their chunks.
- **MCP Protocol**: JSON-RPC 2.0 over stdin/stdout — works with any MCP client.

## Integration

- Uses [keyvalembd](https://github.com/kirill-scherba/keyvalembd) for libSQL-backed key-value store with vector embeddings.
- Uses [anytext/totext](https://github.com/kirill-scherba/anytext) to turn any ingested file (text, DOCX, HTML, PDF, images) into text.
- Uses Ollama for both embeddings (`embeddinggemma:latest`) and answer generation (`phi4-mini`).
- Implements MCP via [mcp-go](https://github.com/mark3labs/mcp-go) SDK.

## Recent Fixes

- 2026-10-04: `rag_ingest` and `rag_ingest_directory` convert `file_path` through the [anytext/totext](https://github.com/kirill-scherba/anytext) library instead of `os.ReadFile`, so any supported document can be ingested directly: text, Markdown, HTML, CSV/TSV, DOCX, PDF (native text plus OCR for scanned pages) and images (OCR). Pages are joined with a blank line before chunking. Depends on `github.com/kirill-scherba/anytext v0.1.0`.
- 2026-05-19: `rag_query` now keeps stderr token streaming disabled by default to avoid blocking MCP clients that do not drain stderr pipes. `rag-cli` can still enable legacy stderr token streaming with `--stream-stderr`.
- 2026-05-19: Embedding writes and semantic search retry `embedder is not ready` during keyvalembd/Ollama cold start before returning an error.

## Author

Kirill Scherba
