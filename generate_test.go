// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"
)

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
