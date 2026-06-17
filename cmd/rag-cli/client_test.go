// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestStreamStderrWithMarkerSimple verifies streaming with marker present.
func TestStreamStderrWithMarkerSimple(t *testing.T) {
	input := "---LLM---Hello world"
	src := strings.NewReader(input)
	var dst bytes.Buffer

	err := streamStderrWithMarker(src, &dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := dst.String()
	if !strings.Contains(out, "Thinking...") {
		t.Errorf("expected 'Thinking...' in output, got: %q", out)
	}
	if !strings.Contains(out, "Hello world") {
		t.Errorf("expected 'Hello world' in output, got: %q", out)
	}
}

// TestStreamStderrWithMarkerSplit verifies marker split across reads.
func TestStreamStderrWithMarkerSplit(t *testing.T) {
	// Use io.Pipe to simulate streaming reads
	pr, pw := io.Pipe()
	var dst bytes.Buffer
	var outErr error
	done := make(chan struct{})

	go func() {
		outErr = streamStderrWithMarker(pr, &dst)
		close(done)
	}()

	// Write prefix, then marker split across two writes
	pw.Write([]byte("prefix "))
	pw.Write([]byte("---LLM"))
	pw.Write([]byte("---token1 token2"))
	pw.Close()

	<-done

	if outErr != nil {
		t.Fatalf("unexpected error: %v", outErr)
	}

	out := dst.String()
	if !strings.Contains(out, "Thinking...") {
		t.Errorf("expected 'Thinking...' in output, got: %q", out)
	}
	if !strings.Contains(out, "prefix ") {
		t.Errorf("expected 'prefix ' in output, got: %q", out)
	}
	if !strings.Contains(out, "token1 token2") {
		t.Errorf("expected 'token1 token2' in output, got: %q", out)
	}
}

// TestStreamStderrWithMarkerNoMarker verifies output without marker.
func TestStreamStderrWithMarkerNoMarker(t *testing.T) {
	input := "some log line"
	src := strings.NewReader(input)
	var dst bytes.Buffer

	err := streamStderrWithMarker(src, &dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := dst.String()
	if strings.Contains(out, "Thinking...") {
		t.Errorf("unexpected 'Thinking...' when no marker present, got: %q", out)
	}
	if !strings.Contains(out, "some log line") {
		t.Errorf("expected 'some log line' in output, got: %q", out)
	}
}

// TestStreamStderrWithMarkerEmpty verifies empty input.
func TestStreamStderrWithMarkerEmpty(t *testing.T) {
	src := strings.NewReader("")
	var dst bytes.Buffer

	err := streamStderrWithMarker(src, &dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dst.Len() != 0 {
		t.Errorf("expected empty output, got: %q", dst.String())
	}
}
