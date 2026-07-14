// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Constants for chunking strategy.
const (
	// minChunkSize is the minimum number of characters per chunk.
	// Shorter chunks get merged with the previous one.
	minChunkSize = 500

	// targetChunkSize is the target number of characters per chunk.
	targetChunkSize = 1200

	// maxChunkSize is the hard maximum character count per chunk.
	maxChunkSize = 2000

	// overlapSentences is the number of sentences to carry over
	// from the previous chunk to maintain context across boundaries.
	overlapSentences = 2
)

// sentenceIter splits text into sentences and yields them one at a time.
type sentenceIter struct {
	text  string
	pos   int
	done  bool
}

// next returns the next sentence (start, end) byte offsets.
// Returns (0, 0, true) when iteration is complete.
func (it *sentenceIter) next() (int, int, bool) {
	if it.done {
		return 0, 0, true
	}

	// Skip leading whitespace
	for it.pos < len(it.text) {
		r, size := utf8.DecodeRuneInString(it.text[it.pos:])
		if !unicode.IsSpace(r) {
			break
		}
		it.pos += size
	}

	if it.pos >= len(it.text) {
		it.done = true
		return 0, 0, true
	}

	start := it.pos
	runes := []rune(it.text[it.pos:])
	runeLen := len(runes)

	for i := 0; i < runeLen; i++ {
		r := runes[i]
		// Check for sentence-ending punctuation
		if r == '.' || r == '!' || r == '?' {
			// Look ahead: end of string or whitespace
			if i+1 >= runeLen || unicode.IsSpace(runes[i+1]) {
				// Include the punctuation and trailing whitespace
				end := i + 1
				for end < runeLen && unicode.IsSpace(runes[end]) {
					end++
				}
				byteEnd := it.pos + len(string(runes[:end]))
				if byteEnd > len(it.text) {
					byteEnd = len(it.text)
				}
				it.pos = byteEnd
				if it.pos >= len(it.text) {
					it.done = true
				}
				return start, it.pos, false
			}
		}
		// Handle ellipsis
		if r == '\u2026' { // …
			end := i + 1
			for end < runeLen && unicode.IsSpace(runes[end]) {
				end++
			}
			byteEnd := it.pos + len(string(runes[:end]))
			if byteEnd > len(it.text) {
				byteEnd = len(it.text)
			}
			it.pos = byteEnd
			if it.pos >= len(it.text) {
				it.done = true
			}
			return start, it.pos, false
		}
	}

	// No sentence boundary found — rest of text is one sentence
	it.pos = len(it.text)
	it.done = true
	return start, it.pos, false
}

// chunkTextSemantic splits text into semantically meaningful chunks.
// It splits by sentences, groups them until targetChunkSize is reached,
// and preserves overlapSentences from the previous chunk for context continuity.
func chunkTextSemantic(text string) []string {
	// Normalize line endings
	text = strings.ReplaceAll(text, "\r\n", "\n")

	if strings.TrimSpace(text) == "" {
		return []string{text}
	}

	// Collect all sentences with their byte spans
	type sentence struct {
		start, end int
		text       string
	}
	var sentences []sentence

	it := &sentenceIter{text: text}
	for {
		start, end, done := it.next()
		if done {
			break
		}
		s := strings.TrimSpace(text[start:end])
		if s == "" {
			continue
		}
		sentences = append(sentences, sentence{start: start, end: end, text: s})
	}

	// If by some quirk we got zero, fall back to line-based splitting
	if len(sentences) == 0 {
		lines := strings.Split(text, "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" {
				sentences = append(sentences, sentence{text: l})
			}
		}
	}

	if len(sentences) == 0 {
		return []string{text}
	}
	// Single sentence — return as-is
	if len(sentences) <= overlapSentences+1 {
		return []string{sentences[0].text}
	}

	var chunks []string
	startIdx := 0

	for startIdx < len(sentences) {
		endIdx := startIdx
		charCount := 0

		// Accumulate sentences until we hit targetChunkSize,
		// or run out of sentences.
		for endIdx < len(sentences) {
			s := sentences[endIdx]
			sLen := utf8.RuneCountInString(s.text)

			// If adding this sentence would exceed maxChunkSize,
			// and we already have at least minChunkSize, stop.
			if charCount > 0 && charCount+sLen > maxChunkSize {
				break
			}

			charCount += sLen
			endIdx++

			// If we've reached or exceeded target, stop accumulating.
			if charCount >= targetChunkSize {
				break
			}
		}

		// Build chunk text from sentences [startIdx, endIdx)
		var chunkBuf strings.Builder
		for i := startIdx; i < endIdx; i++ {
			if chunkBuf.Len() > 0 {
				chunkBuf.WriteByte(' ')
			}
			chunkBuf.WriteString(sentences[i].text)
		}
		chunk := chunkBuf.String()

		// If chunk is too small and there are more sentences, merge with next
		if utf8.RuneCountInString(chunk) < minChunkSize && endIdx < len(sentences) {
			for endIdx < len(sentences) && utf8.RuneCountInString(chunk) < minChunkSize {
				s := sentences[endIdx]
				chunkBuf.WriteByte(' ')
				chunkBuf.WriteString(s.text)
				endIdx++
			}
			chunk = chunkBuf.String()
		}

		// Count actual sentences used after potential extension
		usedSentences := endIdx - startIdx

		if usedSentences <= overlapSentences {
			// No overlap possible — just emit and move on
			chunks = append(chunks, chunk)
			startIdx = endIdx
			continue
		}

		// Add overlap: rewind start by overlapSentences for next chunk
		chunks = append(chunks, chunk)
		startIdx = endIdx - overlapSentences
	}

	// Deduplicate consecutive chunks that are identical (can happen with
	// tiny documents where overlap rewinds too far).
	if len(chunks) > 1 {
		deduped := []string{chunks[0]}
		for i := 1; i < len(chunks); i++ {
			if chunks[i] != chunks[i-1] {
				deduped = append(deduped, chunks[i])
			}
		}
		chunks = deduped
	}

	return chunks
}

// generateDescription creates a short description from text.
// It takes the first up to maxLen characters, breaking at a word boundary.
// If the text is shorter than maxLen, returns it trimmed.
func generateDescription(text string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 150
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	// Normalize whitespace to single spaces for description generation
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= maxLen {
		return string(runes)
	}
	// Find last space before maxLen
	segment := runes[:maxLen]
	lastSpace := -1
	for i := len(segment) - 1; i >= 0; i-- {
		if unicode.IsSpace(segment[i]) {
			lastSpace = i
			break
		}
	}
	if lastSpace > 0 {
		return string(segment[:lastSpace]) + "..."
	}
	return string(segment) + "..."
}


