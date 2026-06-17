// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"testing"
)

// TestMetaKey verifies the metadata key builder.
func TestMetaKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"rag/docs/foo", "rag/docs/foo/meta"},
		{"test", "test/meta"},
		{"", "/meta"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := metaKey(tt.input); got != tt.want {
				t.Errorf("metaKey(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestIsMetaKey verifies meta key pattern matching.
func TestIsMetaKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"rag/docs/foo/meta", true},
		{"rag/docs/foo/chunk/0000", false},
		{"rag/docs/foo", false},
		{"meta", false},
		{"/meta", true},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := isMetaKey(tt.key); got != tt.want {
				t.Errorf("isMetaKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

// TestIsChunkKey verifies chunk key pattern matching.
func TestIsChunkKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"rag/docs/foo/chunk/0000", true},
		{"rag/docs/foo/chunk/", true},
		{"rag/docs/foo/meta", false},
		{"rag/docs/foo", false},
		{"chunk", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := isChunkKey(tt.key); got != tt.want {
				t.Errorf("isChunkKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

// TestRagResultMarshal verifies JSON round-trip for ragResult.
func TestRagResultMarshal(t *testing.T) {
	original := ragResult{
		Key:      "test/key",
		Text:     "some text",
		Score:    0.95,
		Checksum: "abc123",
		Index:    5,
		Total:    10,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ragResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Key != original.Key {
		t.Errorf("Key mismatch: got %q, want %q", decoded.Key, original.Key)
	}
	if decoded.Text != original.Text {
		t.Errorf("Text mismatch: got %q, want %q", decoded.Text, original.Text)
	}
	if decoded.Score != original.Score {
		t.Errorf("Score mismatch: got %f, want %f", decoded.Score, original.Score)
	}
	if decoded.Checksum != original.Checksum {
		t.Errorf("Checksum mismatch: got %q, want %q", decoded.Checksum, original.Checksum)
	}
	if decoded.Index != original.Index {
		t.Errorf("Index mismatch: got %d, want %d", decoded.Index, original.Index)
	}
	if decoded.Total != original.Total {
		t.Errorf("Total mismatch: got %d, want %d", decoded.Total, original.Total)
	}
}

// TestDocMetaMarshal verifies JSON round-trip for docMeta.
func TestDocMetaMarshal(t *testing.T) {
	original := docMeta{
		Description: "Test doc",
		NumChunks:   5,
		Source:      "/path/to/file",
		Stored:      "2026-01-01T00:00:00Z",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded docMeta
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Description != original.Description {
		t.Errorf("Description mismatch")
	}
	if decoded.NumChunks != original.NumChunks {
		t.Errorf("NumChunks mismatch")
	}
	if decoded.Source != original.Source {
		t.Errorf("Source mismatch")
	}
	if decoded.Stored != original.Stored {
		t.Errorf("Stored mismatch")
	}
}