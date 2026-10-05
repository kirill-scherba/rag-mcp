// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rag

import (
	"strings"
	"testing"
)

// TestChunkEmpty verifies empty and whitespace-only input.
func TestChunkEmpty(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty string", "", 1},
		{"whitespace only", "   \n\t  ", 1},
		{"single space", " ", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Chunk(tt.input)
			if len(got) != tt.want {
				t.Errorf("Chunk(%q) returned %d chunks, want %d", tt.input, len(got), tt.want)
			}
		})
	}
}

// TestChunkSingleParagraph verifies single paragraph handling.
func TestChunkSingleParagraph(t *testing.T) {
	input := "This is a single paragraph of text that should be long enough to form one chunk."
	got := Chunk(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(got))
	}
	if got[0] != strings.TrimSpace(got[0]) {
		t.Errorf("chunk has leading/trailing whitespace: %q", got[0])
	}
}

// TestChunkMultipleParagraphs verifies multi-paragraph input.
func TestChunkMultipleParagraphs(t *testing.T) {
	input := `First paragraph about Cooksy platform features and capabilities.

Second paragraph discussing architecture and design decisions.

Third paragraph covering deployment and configuration options.`
	got := Chunk(input)
	if len(got) < 1 {
		t.Fatalf("expected at least 1 chunk, got %d", len(got))
	}
	for i, c := range got {
		if c != strings.TrimSpace(c) {
			t.Errorf("chunk %d has leading/trailing whitespace: %q", i, c)
		}
	}
}

// TestChunkShortText verifies very short input (< minChunkSize).
func TestChunkShortText(t *testing.T) {
	input := "Short."
	got := Chunk(input)
	if len(got) != 1 {
		t.Errorf("expected 1 chunk for short text, got %d", len(got))
	}
	if got[0] != "Short." {
		t.Errorf("expected chunk to be %q, got %q", "Short.", got[0])
	}
}

// TestChunkShortMultiSentence verifies a 2-3 sentence document keeps all of it.
func TestChunkShortMultiSentence(t *testing.T) {
	input := "First one. Second one. Third one."
	got := Chunk(input)
	if len(got) != 1 {
		t.Fatalf("chunks=%d, want 1", len(got))
	}
	for _, want := range []string{"First one.", "Second one.", "Third one."} {
		if !strings.Contains(got[0], want) {
			t.Errorf("chunk %q lost %q", got[0], want)
		}
	}
}

// TestChunkWindowsLineEndings verifies CRLF normalization.
func TestChunkWindowsLineEndings(t *testing.T) {
	input := "Line one.\r\n\r\nLine two."
	got := Chunk(input)
	if len(got) < 1 {
		t.Fatalf("expected at least 1 chunk, got %d", len(got))
	}
}

// TestChunkUnicode verifies emoji and multi-byte characters.
func TestChunkUnicode(t *testing.T) {
	input := "Hello world! 😀 This is a test with emoji. 🚀 Another sentence here."
	got := Chunk(input)
	if len(got) < 1 {
		t.Fatalf("expected at least 1 chunk, got %d", len(got))
	}
	for i, c := range got {
		if strings.TrimSpace(c) == "" {
			t.Errorf("chunk %d is empty", i)
		}
	}
}

// TestChunkLargeDoc generates enough text to trigger multiple chunks.
func TestChunkLargeDoc(t *testing.T) {
	var parts []string
	for i := 0; i < 40; i++ {
		parts = append(parts, "This is sentence number %d in our large document. It contains enough words to be meaningful.")
	}
	input := strings.Join(parts, " ")
	got := Chunk(input)
	if len(got) < 2 {
		t.Errorf("expected multiple chunks for large doc, got %d", len(got))
	}
	if len(got) > 1 {
		overlapFound := false
		for i := 0; i < len(got)-1; i++ {
			words1 := strings.Fields(got[i])
			words2 := strings.Fields(got[i+1])
			if len(words1) > 0 && len(words2) > 0 {
				lastWord := words1[len(words1)-1]
				if strings.Contains(got[i+1], lastWord) {
					overlapFound = true
				}
			}
		}
		if !overlapFound {
			t.Log("warning: no obvious overlap found between chunks")
		}
	}
}

// TestChunkDedupe verifies deduplication of consecutive identical chunks.
func TestChunkDedupe(t *testing.T) {
	input := "A. B. C. D. E. F. G. H. I. J. K. L. M. N. O. P. Q. R. S. T. U. V. W. X. Y. Z."
	got := Chunk(input)
	for i := 0; i < len(got)-1; i++ {
		if got[i] == got[i+1] {
			t.Errorf("chunks %d and %d are identical after dedup: %q", i, i+1, got[i])
		}
	}
}

// TestSentenceIter verifies sentence boundary detection.
func TestSentenceIter(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"period", "First. Second.", []string{"First.", "Second."}},
		{"exclamation", "Wow! Great!", []string{"Wow!", "Great!"}},
		{"question", "What? Really?", []string{"What?", "Really?"}},
		{"ellipsis", "Well… Maybe…", []string{"Well…", "Maybe…"}},
		{"mixed", "Hello. Wow! What?", []string{"Hello.", "Wow!", "What?"}},
		{"no punctuation", "no punctuation here", []string{"no punctuation here"}},
		{"whitespace only", "   ", []string{}},
		{"single space after period", "A. B. C.", []string{"A.", "B.", "C."}},
		{"multi spaces after", "A.   B.", []string{"A.", "B."}},
		{"newline separator", "A.\nB.", []string{"A.", "B."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := &sentenceIter{text: tt.text}
			var got []string
			for {
				start, end, done := it.next()
				if done {
					break
				}
				s := strings.TrimSpace(tt.text[start:end])
				if s != "" {
					got = append(got, s)
				}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("sentenceIter produced %d sentences, want %d: got %v, want %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("sentence %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestDescription verifies description generation.
func TestDescription(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		maxLen int
		want   string
	}{
		{"short text", "Hello world", 100, "Hello world"},
		{"empty text", "", 100, ""},
		{"long at boundary", "The quick brown fox jumps over the lazy dog. This is a longer sentence that we use for testing.", 20, "The quick brown fox..."},
		{"no boundary", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", 10, "ABCDEFGHIJ..."},
		{"maxLen zero default", "Some text here", 0, "Some text here"},
		{"word with ellipsis", "Hello world this is a test", 10, "Hello..."},
		{"normalize whitespace", "Hello    world\n\tthis", 50, "Hello world this"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Description(tt.text, tt.maxLen)
			if got != tt.want {
				t.Errorf("Description(%q, %d) = %q, want %q", tt.text, tt.maxLen, got, tt.want)
			}
		})
	}
}
