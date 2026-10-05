// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeScript(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "extractor.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractWithoutCommandReadsRaw(t *testing.T) {
	t.Setenv(extractorCmdEnv, "")
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("plain text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := extractFileText(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "plain text\n" {
		t.Errorf("got %q", got)
	}
}

func TestExtractWithCommand(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "doc.bin")
	if err := os.WriteFile(doc, []byte("RAW"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := writeScript(t, dir, `echo "EXTRACTED:"; cat "$1"`)
	t.Setenv(extractorCmdEnv, script)

	got, err := extractFileText(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "EXTRACTED:") || !strings.Contains(got, "RAW") {
		t.Errorf("extractor not used: %q", got)
	}
}

func TestExtractWithPlaceholder(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(doc, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := writeScript(t, dir, `echo "GOT:$1"`)
	t.Setenv(extractorCmdEnv, script+" {file}")

	got, err := extractFileText(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "GOT:"+doc) {
		t.Errorf("placeholder not substituted: %q", got)
	}
}

func TestExtractCommandFailure(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(doc, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(extractorCmdEnv, "false") // exits non-zero

	if _, err := extractFileText(context.Background(), doc); err == nil {
		t.Fatal("expected an error from a failing extractor")
	}
}

func TestTokenizeCommand(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`pdftotext {file}`, []string{"pdftotext", "{file}"}},
		{`pdftotext -layout {file}`, []string{"pdftotext", "-layout", "{file}"}},
		{`prog "a b" c`, []string{"prog", "a b", "c"}},
		{`prog 'a b'`, []string{"prog", "a b"}},
		{`prog a\ b`, []string{"prog", "a b"}},
		{`  spaced   out  `, []string{"spaced", "out"}},
	}
	for _, c := range cases {
		got, err := tokenizeCommand(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %v, want %v", c.in, got, c.want)
		}
	}
	if _, err := tokenizeCommand(`unbalanced "quote`); err == nil {
		t.Error("expected an error for unbalanced quotes")
	}
}
