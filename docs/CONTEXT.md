# rag-mcp — Context

## Overview

rag-mcp is an MCP (Model Context Protocol) server that provides Retrieval-Augmented Generation (RAG) capabilities. It allows ingesting documents, storing them as vector embeddings, and answering questions by combining semantic search with LLM-generated answers.

## Purpose

- Provide a pluggable RAG knowledge base accessible via MCP tools.
- Enable AI assistants to store and retrieve contextual knowledge from documents.
- Serve as the knowledge backend for the Cooksy project and other applications.

## Key Features

- **Document Ingestion**: Split text into chunks, generate embeddings, store in libSQL.
- **Semantic Search**: Find relevant document chunks by meaning (not keywords).
- **LLM Generation**: Answer questions using retrieved context + Ollama LLM.
- **MCP Protocol**: JSON-RPC 2.0 over stdin/stdout — works with any MCP client.

## Integration

- Uses [keyvalembd](https://github.com/kirill-scherba/keyvalembd) for libSQL-backed key-value store with vector embeddings.
- Uses Ollama for both embeddings (`embeddinggemma:latest`) and answer generation (`qwen2.5-7B`).
- Implements MCP via [mcp-go](https://github.com/mark3labs/mcp-go) SDK.

## Author

Kirill Scherba