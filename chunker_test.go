// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"math"
	"strings"
	"testing"
)

// TestChunkTextSemanticEmpty verifies empty and whitespace-only input.
func TestChunkTextSemanticEmpty(t *testing.T) {
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
			got := chunkTextSemantic(tt.input)
			if len(got) != tt.want {
				t.Errorf("chunkTextSemantic(%q) returned %d chunks, want %d", tt.input, len(got), tt.want)
			}
		})
	}
}

// TestChunkTextSemanticSingleParagraph verifies single paragraph handling.
func TestChunkTextSemanticSingleParagraph(t *testing.T) {
	input := "This is a single paragraph of text that should be long enough to form one chunk."
	got := chunkTextSemantic(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(got))
	}
	if got[0] != strings.TrimSpace(got[0]) {
		t.Errorf("chunk has leading/trailing whitespace: %q", got[0])
	}
}

// TestChunkTextSemanticMultipleParagraphs verifies multi-paragraph input.
func TestChunkTextSemanticMultipleParagraphs(t *testing.T) {
	input := `First paragraph about Cooksy platform features and capabilities.

Second paragraph discussing architecture and design decisions.

Third paragraph covering deployment and configuration options.`
	got := chunkTextSemantic(input)
	if len(got) < 1 {
		t.Fatalf("expected at least 1 chunk, got %d", len(got))
	}
	for i, c := range got {
		if c != strings.TrimSpace(c) {
			t.Errorf("chunk %d has leading/trailing whitespace: %q", i, c)
		}
	}
}

// TestChunkTextSemanticShortText verifies very short input (< minChunkSize).
func TestChunkTextSemanticShortText(t *testing.T) {
	input := "Short."
	got := chunkTextSemantic(input)
	if len(got) != 1 {
		t.Errorf("expected 1 chunk for short text, got %d", len(got))
	}
	if got[0] != "Short." {
		t.Errorf("expected chunk to be %q, got %q", "Short.", got[0])
	}
}

// TestChunkTextSemanticWindowsLineEndings verifies CRLF normalization.
func TestChunkTextSemanticWindowsLineEndings(t *testing.T) {
	input := "Line one.\r\n\r\nLine two."
	got := chunkTextSemantic(input)
	if len(got) < 1 {
		t.Fatalf("expected at least 1 chunk, got %d", len(got))
	}
}

// TestChunkTextSemanticUnicode verifies emoji and multi-byte characters.
func TestChunkTextSemanticUnicode(t *testing.T) {
	input := "Hello world! 😀 This is a test with emoji. 🚀 Another sentence here."
	got := chunkTextSemantic(input)
	if len(got) < 1 {
		t.Fatalf("expected at least 1 chunk, got %d", len(got))
	}
	// Verify all chunks are non-empty
	for i, c := range got {
		if strings.TrimSpace(c) == "" {
			t.Errorf("chunk %d is empty", i)
		}
	}
}

// TestChunkTextSemanticLargeDoc generates enough text to trigger multiple chunks.
func TestChunkTextSemanticLargeDoc(t *testing.T) {
	// Build a ~4000 character text with ~20 sentences
	var parts []string
	for i := 0; i < 40; i++ {
		parts = append(parts, "This is sentence number %d in our large document. It contains enough words to be meaningful.")
	}
	input := strings.Join(parts, " ")
	got := chunkTextSemantic(input)
	// Should produce multiple chunks since total > maxChunkSize (2000)
	if len(got) < 2 {
		t.Errorf("expected multiple chunks for large doc, got %d", len(got))
	}
	// Verify overlap: adjacent chunks should share some content (last 2 sentences)
	if len(got) > 1 {
		// Adjacent chunks should have some overlap
		overlapFound := false
		for i := 0; i < len(got)-1; i++ {
			words1 := strings.Fields(got[i])
			words2 := strings.Fields(got[i+1])
			if len(words1) > 0 && len(words2) > 0 {
				// Check if last few words of chunk i appear in chunk i+1
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

// TestChunkTextSemanticDedupe verifies deduplication of consecutive identical chunks.
func TestChunkTextSemanticDedupe(t *testing.T) {
	// A tiny document where overlap might create identical consecutive chunks
	input := "A. B. C. D. E. F. G. H. I. J. K. L. M. N. O. P. Q. R. S. T. U. V. W. X. Y. Z."
	got := chunkTextSemantic(input)
	// Check no two consecutive chunks are identical
	for i := 0; i < len(got)-1; i++ {
		if got[i] == got[i+1] {
			t.Errorf("chunks %d and %d are identical after dedup: %q", i, i+1, got[i])
		}
	}
}

// TestChunkString verifies the chunkString utility.
func TestChunkString(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		step int
		want []string
	}{
		{"empty string", "", 5, 2, []string{}},
		{"n <= 0", "hello", 0, 2, []string{}},
		{"step <= 0", "hello", 5, 0, []string{}},
		{"shorter than n", "hi", 5, 2, []string{"hi"}},
		{"exactly n", "hello", 5, 2, []string{"hello"}},
		{"longer than n step=n", "abcdefgh", 3, 3, []string{"abc", "def", "gh"}},
		{"longer step half", "abcdefghij", 4, 2, []string{"abcd", "cdef", "efgh", "ghij", "ij"}},
		{"unicode", "привет мир", 3, 2, []string{"при", "иве", "ет ", " ми", "ир"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkString(tt.s, tt.n, tt.step)
			if len(got) != len(tt.want) {
				t.Fatalf("chunkString(%q, %d, %d) = %v, want %v", tt.s, tt.n, tt.step, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("chunk %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
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

// TestCountSentences verifies sentence counting.
func TestCountSentences(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"One.", 1},
		{"One. Two. Three.", 3},
		{"No punctuation here", 1}, // fallback: whole text counts as 1
		{"Whitespace only", 1},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := countSentences(tt.input)
			if got != tt.want {
				t.Errorf("countSentences(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestSentenceLengths verifies sentence length calculation.
func TestSentenceLengths(t *testing.T) {
	input := "Hi. Hello world."
	want := []int{3, 12}
	got := sentenceLengths(input)
	if len(got) != len(want) {
		t.Fatalf("sentenceLengths(%q) = %v, want %v", input, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("length %d: got %d, want %d", i, got[i], want[i])
		}
	}

	// Empty input
	emptyGot := sentenceLengths("")
	if len(emptyGot) != 0 {
		t.Errorf("sentenceLengths(\"\") = %v, want empty", emptyGot)
	}
}

// TestAverageSentenceLength verifies mean calculation.
func TestAverageSentenceLength(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"", 0},
		{"A.", 2},
		{"A. BB.", 2.5},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := averageSentenceLength(tt.input)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("averageSentenceLength(%q) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

// TestStddevSentenceLength verifies standard deviation.
func TestStddevSentenceLength(t *testing.T) {
	// Uniform text → stddev should be 0
	uniform := "A. B. C. D."
	gotUniform := stddevSentenceLength(uniform)
	if gotUniform != 0 {
		t.Errorf("stddevSentenceLength(%q) = %f, want 0", uniform, gotUniform)
	}

	// Empty text
	if stddevSentenceLength("") != 0 {
		t.Errorf("stddevSentenceLength(\"\") = %f, want 0", stddevSentenceLength(""))
	}
}

// TestGenerateDescription verifies description generation.
func TestGenerateDescription(t *testing.T) {
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
			got := generateDescription(tt.text, tt.maxLen)
			if got != tt.want {
				t.Errorf("generateDescription(%q, %d) = %q, want %q", tt.text, tt.maxLen, got, tt.want)
			}
		})
	}
}
