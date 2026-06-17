// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kirill-scherba/keyvalembd"
	"github.com/kirill-scherba/s3lite"
)

// ragResult holds a single chunk result from RAG operations.
type ragResult struct {
	Key      string  `json:"key"`
	Text     string  `json:"text"`
	Score    float64 `json:"score"`
	Checksum string  `json:"checksum"`
	Index    int     `json:"index"`
	Total    int     `json:"total"`
}

// Serialize access to keyvalembd so that tools/call handlers execute
// sequentially, avoiding race conditions when multiple requests arrive
// on the same stdin stream (e.g. ingest → query → delete in one pipe).
var mu sync.Mutex

const (
	embedderRetryAttempts = 40
	embedderRetryDelay    = 250 * time.Millisecond
)

// metaSuffix is the key suffix for document metadata entries.
const metaSuffix = "meta"

// docMeta holds metadata for a document (stored at doc_key/meta).
type docMeta struct {
	Description string `json:"description"`
	NumChunks   int    `json:"num_chunks"`
	Source      string `json:"source,omitempty"`
	Stored      string `json:"stored"`
}

// metaKey returns the metadata key for a document.
func metaKey(docKey string) string { return docKey + "/" + metaSuffix }

// isMetaKey reports whether a key is a document metadata entry.
func isMetaKey(key string) bool { return strings.HasSuffix(key, "/"+metaSuffix) }

// isChunkKey reports whether a key is a document chunk entry.
func isChunkKey(key string) bool { return strings.Contains(key, "/chunk/") }

// deleteOldChunks removes all existing chunks and metadata for a document.
func deleteOldChunks(ctx context.Context, kv *keyvalembd.KeyValueEmbd, docKey string) (int, error) {
	deleted := 0
	for key := range kv.List(docKey) {
		if err := kv.Del(key); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// storeMeta saves document metadata.
func storeMeta(ctx context.Context, kv *keyvalembd.KeyValueEmbd, docKey string, meta docMeta) error {
	m, _ := json.Marshal(meta)
	_, err := withEmbedderRetry(ctx, func() (*s3lite.ObjectInfo, error) {
		return kv.SetWithEmbedding(metaKey(docKey), m, "")
	})
	return err
}

// loadMeta loads document metadata. Returns nil if not found.
func loadMeta(kv *keyvalembd.KeyValueEmbd, docKey string) *docMeta {
	data, err := kv.Get(metaKey(docKey))
	if err != nil || len(data) == 0 {
		return nil
	}
	var meta docMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}
	return &meta
}

func isEmbedderNotReady(err error) bool {
	return err != nil && strings.Contains(err.Error(), "embedder is not ready")
}

func withEmbedderRetry[T any](ctx context.Context, op func() (T, error)) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt < embedderRetryAttempts; attempt++ {
		result, err := op()
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !isEmbedderNotReady(err) {
			return zero, err
		}
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(embedderRetryDelay):
		}
	}
	return zero, lastErr
}

// storeChunks stores chunk texts with embeddings for a document.
// Each chunk gets a sequential key, computed checksum, and the embedding vector.
// Returns formatted result lines (one per chunk) and an error.
// Fails on the first chunk storage error.
func storeChunks(
	ctx context.Context,
	kv *keyvalembd.KeyValueEmbd,
	docKey string,
	chunks []string,
	source string,
) ([]string, error) {
	var results []string
	for i, chunk := range chunks {
		chunkKey := fmt.Sprintf("%s/chunk/%04d", docKey, i)
		checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(chunk)))
		val := map[string]interface{}{
			"index":    i,
			"total":    len(chunks),
			"checksum": checksum,
			"text":     chunk,
			"doc_key":  docKey,
			"stored":   time.Now().UTC().Format(time.RFC3339),
		}
		if source != "" {
			val["source"] = source
		}
		valJSON, _ := json.Marshal(val)
		info, err := withEmbedderRetry(ctx, func() (*s3lite.ObjectInfo, error) {
			return kv.SetWithEmbedding(chunkKey, valJSON, chunk)
		})
		if err != nil {
			return results, fmt.Errorf("Error storing chunk %d/%d: %w", i+1, len(chunks), err)
		}
		results = append(results, fmt.Sprintf(
			"  chunk %d/%d: key=%s, size=%d", i+1, len(chunks), info.Checksum, info.ContentLength))
	}
	return results, nil
}
