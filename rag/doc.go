// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rag is the reusable core of the RAG engine: sentence-aware chunking,
// key-value + embedding storage over keyvalembd, semantic and exact keyword
// search, and file → text extraction via an optional external command.
//
// The rag-mcp MCP server is a thin wrapper over this package; other consumers
// (for example the AnyText GUI server) import it directly.
package rag
