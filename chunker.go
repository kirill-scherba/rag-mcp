// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
)

// minChunkSize is the minimum number of characters per chunk.
// Shorter chunks get merged with the previous one.
const minChunkSize = 100

// chunkText splits text into chunks by paragraphs (double newline).
// Paragraphs shorter than minChunkSize are merged with the previous chunk.
func chunkText(text string) []string {
	// Normalize line endings
	text = strings.ReplaceAll(text, "\r\n", "\n")

	// Split by double newline (paragraphs)
	paragraphs := strings.Split(text, "\n\n")

	var chunks []string
	var current strings.Builder

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// If current buffer is empty, start a new chunk
		if current.Len() == 0 {
			current.WriteString(p)
			continue
		}

		// If current chunk is still short, append paragraph
		if current.Len() < minChunkSize {
			current.WriteString("\n\n")
			current.WriteString(p)
			continue
		}

		// Current chunk is long enough, flush it
		chunks = append(chunks, current.String())
		current.Reset()
		current.WriteString(p)
	}

	// Flush the last chunk
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	// If no chunks were produced (e.g. empty input), return a single empty chunk
	if len(chunks) == 0 {
		return []string{text}
	}

	return chunks
}