// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"strings"

	"github.com/kirill-scherba/anytext"
)

// extractFileText converts any supported file into plain text with the totext
// library: text formats, DOCX/HTML/CSV, images via OCR, and PDF (native text
// plus OCR for scanned pages). Pages are joined with a blank line so chunking
// keeps a natural boundary between them.
func extractFileText(ctx context.Context, path string) (string, error) {
	doc, err := totext.Extract(ctx, path, totext.Options{})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, p := range doc.Pages {
		if p.Err != nil {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(p.Text)
	}
	return b.String(), nil
}
