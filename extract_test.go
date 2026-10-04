// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractFileTextPlain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("hello rag\nsecond line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := extractFileText(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "hello rag") || !strings.Contains(got, "second line") {
		t.Errorf("got %q", got)
	}
}

func TestExtractFileTextUnsupported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0x03}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := extractFileText(context.Background(), path); err == nil {
		t.Error("expected an error for an unsupported format")
	}
}
