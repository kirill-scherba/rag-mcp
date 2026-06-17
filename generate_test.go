// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestParseOllamaResponseSingleJSON verifies parsing a single JSON object response.
func TestParseOllamaResponseSingleJSON(t *testing.T) {
	resp := OllamaChatResponse{
		Message: &OllamaChatMessage{Content: "Hello world"},
		Done:    true,
	}
	data, _ := json.Marshal(resp)
	got, err := parseOllamaResponse(data)
	if err != nil {
		t.Fatalf("parseOllamaResponse single JSON: %v", err)
	}
	if got != "Hello world" {
		t.Errorf("parseOllamaResponse single JSON: got %q, want %q", got, "Hello world")
	}
}

// TestParseOllamaResponseNDJSON verifies parsing a streaming NDJSON response.
func TestParseOllamaResponseNDJSON(t *testing.T) {
	chunks := []OllamaChatResponse{
		{Message: &OllamaChatMessage{Content: "Hello"}},
		{Message: &OllamaChatMessage{Content: " world"}},
		{Message: &OllamaChatMessage{Content: "!"}, Done: true},
	}
	var lines []byte
	for i, c := range chunks {
		b, _ := json.Marshal(c)
		lines = append(lines, b...)
		if i < len(chunks)-1 {
			lines = append(lines, '\n')
		}
	}
	got, err := parseOllamaResponse(lines)
	if err != nil {
		t.Fatalf("parseOllamaResponse NDJSON: %v", err)
	}
	if got != "Hello world!" {
		t.Errorf("parseOllamaResponse NDJSON: got %q, want %q", got, "Hello world!")
	}
}

// TestParseOllamaResponseMalformedInput verifies graceful handling of malformed data.
func TestParseOllamaResponseMalformedInput(t *testing.T) {
	// Malformed JSON lines interspersed with valid ones
	data := []byte(`{bad json}
{"message": {"content": "valid"}}
also bad
{"done": true, "message": {"content": ""}}
`)
	got, err := parseOllamaResponse(data)
	if err != nil {
		t.Fatalf("parseOllamaResponse malformed: %v", err)
	}
	if got != "valid" {
		t.Errorf("parseOllamaResponse malformed: got %q, want %q", got, "valid")
	}
}

// TestParseOllamaResponseEmptyInput verifies empty input returns error.
func TestParseOllamaResponseEmptyInput(t *testing.T) {
	got, err := parseOllamaResponse([]byte(""))
	if err == nil {
		t.Fatalf("expected error for empty input, got %q", got)
	}
}

// TestParseOllamaResponseOnlyDoneTrue verifies a response with done=true but no message content.
func TestParseOllamaResponseOnlyDoneTrue(t *testing.T) {
	resp := OllamaChatResponse{Done: true}
	data, _ := json.Marshal(resp)
	got, err := parseOllamaResponse(data)
	if err == nil {
		t.Fatalf("expected error for done-only response, got %q", got)
	}
}

// TestBuildRAGPromptStrict verifies the strict prompt construction.
func TestBuildRAGPromptStrict(t *testing.T) {
	chunks := []ragResult{
		{Text: "func Foo() error", Score: 0.9},
		{Text: "func Bar(x int)", Score: 0.8},
	}
	msgs, err := buildRAGPrompt(chunks, "Find Foo", "strict")
	if err != nil {
		t.Fatalf("buildRAGPrompt strict: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected role 'system', got %q", msgs[0].Role)
	}
	if msgs[1].Role != "user" {
		t.Errorf("expected role 'user', got %q", msgs[1].Role)
	}
	if msgs[1].Content != "Find Foo" {
		t.Errorf("expected user content 'Find Foo', got %q", msgs[1].Content)
	}
	if !strings.Contains(msgs[0].Content, "func Foo() error") {
		t.Errorf("expected system prompt to contain chunk text")
	}
	if !strings.Contains(msgs[0].Content, "strict copy-paste assistant") {
		t.Errorf("expected system prompt to reference strict mode")
	}
}

// TestBuildRAGPromptCreative verifies the creative prompt construction.
func TestBuildRAGPromptCreative(t *testing.T) {
	chunks := []ragResult{{Text: "Cooksy is a recipe app.", Score: 0.95}}
	msgs, err := buildRAGPrompt(chunks, "What is Cooksy?", "creative")
	if err != nil {
		t.Fatalf("buildRAGPrompt creative: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "knowledgeable and helpful assistant") {
		t.Errorf("expected system prompt to reference creative mode")
	}
}

// TestBuildRAGPromptEmptyChunks verifies handling of empty chunk list.
func TestBuildRAGPromptEmptyChunks(t *testing.T) {
	msgs, err := buildRAGPrompt([]ragResult{}, "Empty test", "strict")
	if err != nil {
		t.Fatalf("buildRAGPrompt empty: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "FRAGMENTS:") {
		t.Errorf("expected system prompt to contain FRAGMENTS section")
	}
}

// TestBoolPtr verifies the boolPtr helper.
func TestBoolPtr(t *testing.T) {
	if *boolPtr(true) != true {
		t.Error("boolPtr(true) should return pointer to true")
	}
	if *boolPtr(false) != false {
		t.Error("boolPtr(false) should return pointer to false")
	}
}
