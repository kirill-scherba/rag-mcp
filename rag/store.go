// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rag

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kirill-scherba/keyvalembd"
	"github.com/kirill-scherba/s3lite"
)

const (
	embedderRetryAttempts = 40
	embedderRetryDelay    = 250 * time.Millisecond
)

// metaSuffix is the key suffix for document metadata entries.
const metaSuffix = "meta"

// DocMeta holds metadata for a document (stored at <doc_key>/meta).
type DocMeta struct {
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

// Result is a single chunk result from a semantic search.
type Result struct {
	Key      string  `json:"key"`
	Text     string  `json:"text"`
	Score    float64 `json:"score"`
	Checksum string  `json:"checksum"`
	Index    int     `json:"index"`
	Total    int     `json:"total"`
}

// Doc is a document entry for listing.
type Doc struct {
	Key         string
	Description string
	NumChunks   int
	Stored      string
}

// IngestStats summarises an ingest operation.
type IngestStats struct {
	NumChunks   int
	DeletedOld  int
	Description string
	Source      string
}

// Store is a RAG database: key-value storage with embeddings plus a keyword
// index. It is NOT safe for concurrent use; the caller must serialize calls.
type Store struct {
	dbPath string
	kv     *keyvalembd.KeyValueEmbd
	ti     *TextIndex
}

// Open opens (or creates) a RAG database at dbPath.
func Open(dbPath string) (*Store, error) {
	kv, err := keyvalembd.New(dbPath)
	if err != nil {
		return nil, err
	}
	ti, err := OpenTextIndex(dbPath)
	if err != nil {
		kv.Close()
		return nil, err
	}
	return &Store{dbPath: dbPath, kv: kv, ti: ti}, nil
}

// Close releases database resources.
func (s *Store) Close() {
	if s == nil {
		return
	}
	if s.ti != nil {
		s.ti.Close()
	}
	if s.kv != nil {
		s.kv.Close()
	}
}

// DBPath returns the database path.
func (s *Store) DBPath() string { return s.dbPath }

// KV exposes the underlying key-value store (used by tests and advanced callers).
func (s *Store) KV() *keyvalembd.KeyValueEmbd { return s.kv }

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

// loadMeta loads document metadata. Returns nil if not found.
func (s *Store) loadMeta(docKey string) *DocMeta {
	data, err := s.kv.Get(metaKey(docKey))
	if err != nil || len(data) == 0 {
		return nil
	}
	var meta DocMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}
	return &meta
}

// LoadMeta loads document metadata. Returns nil if not found.
func (s *Store) LoadMeta(docKey string) *DocMeta { return s.loadMeta(docKey) }

// storeMeta saves document metadata.
func (s *Store) storeMeta(ctx context.Context, docKey string, meta DocMeta) error {
	m, _ := json.Marshal(meta)
	_, err := withEmbedderRetry(ctx, func() (*s3lite.ObjectInfo, error) {
		return s.kv.SetWithEmbedding(metaKey(docKey), m, "")
	})
	return err
}

// allKeys returns every leaf key under a prefix. List collapses child keys
// into folder entries (e.g. "doc/chunk/"), and Del on a folder entry is a
// no-op, so deletion must recurse to the actual leaf keys.
func (s *Store) allKeys(prefix string) []string {
	var out []string
	for key := range s.kv.List(prefix) {
		if strings.HasSuffix(key, "/") {
			out = append(out, s.allKeys(key)...)
		} else {
			out = append(out, key)
		}
	}
	return out
}

// deleteOldChunks removes all existing chunks and metadata for a document.
func (s *Store) deleteOldChunks(ctx context.Context, docKey string) (int, error) {
	deleted := 0
	for _, key := range s.allKeys(docKey) {
		if err := s.kv.Del(key); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// storeChunks stores chunk texts with embeddings for a document.
func (s *Store) storeChunks(ctx context.Context, docKey string, chunks []string, source string) error {
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
		if _, err := withEmbedderRetry(ctx, func() (*s3lite.ObjectInfo, error) {
			return s.kv.SetWithEmbedding(chunkKey, valJSON, chunk)
		}); err != nil {
			return fmt.Errorf("store chunk %d/%d: %w", i+1, len(chunks), err)
		}
	}
	return nil
}

// Ingest chunks text, generates embeddings and stores it under docKey, replacing
// any previous content for that key. An empty description is auto-generated.
func (s *Store) Ingest(ctx context.Context, docKey, text, source, description string) (IngestStats, error) {
	chunks := Chunk(text)
	if len(chunks) == 0 {
		return IngestStats{}, fmt.Errorf("no chunks generated for %q", docKey)
	}
	deleted, err := s.deleteOldChunks(ctx, docKey)
	if err != nil {
		return IngestStats{}, fmt.Errorf("delete old chunks: %w", err)
	}
	desc := description
	if desc == "" {
		desc = Description(text, 150)
	}
	if err := s.storeChunks(ctx, docKey, chunks, source); err != nil {
		return IngestStats{}, err
	}
	if err := s.storeMeta(ctx, docKey, DocMeta{
		Description: desc,
		NumChunks:   len(chunks),
		Source:      source,
		Stored:      time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return IngestStats{}, err
	}
	return IngestStats{NumChunks: len(chunks), DeletedOld: deleted, Description: desc, Source: source}, nil
}

// IngestFile extracts a file to text (via EXTRACTOR_CMD or a raw read) and ingests it.
func (s *Store) IngestFile(ctx context.Context, docKey, filePath, description string) (IngestStats, error) {
	text, err := ExtractFileText(ctx, filePath)
	if err != nil {
		return IngestStats{}, err
	}
	return s.Ingest(ctx, docKey, text, filePath, description)
}

// Search performs semantic search and returns the top-K chunks.
func (s *Store) Search(ctx context.Context, query string, topK int) ([]Result, error) {
	res, err := withEmbedderRetry(ctx, func() ([]keyvalembd.SearchResult, error) {
		return s.kv.SearchSemantic(query, topK)
	})
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(res))
	for _, r := range res {
		out = append(out, Result{Key: r.Key, Text: r.Text, Score: r.Score})
	}
	return out, nil
}

// Find performs exact keyword search (Unicode case-insensitive).
func (s *Store) Find(keyword string, limit int) ([]FindResult, error) {
	return s.ti.Find(keyword, limit)
}

// collectDocs recursively collects document keys under a prefix.
func (s *Store) collectDocs(prefix string, out map[string]struct{}) {
	for key := range s.kv.List(prefix) {
		if isMetaKey(key) {
			docKey := strings.TrimSuffix(key, "/"+metaSuffix)
			out[docKey] = struct{}{}
		} else if isChunkKey(key) {
			parts := strings.Split(key, "/chunk/")
			if len(parts) > 0 {
				out[parts[0]] = struct{}{}
			}
		} else if strings.HasSuffix(key, "/") {
			s.collectDocs(key, out)
		} else {
			out[key] = struct{}{}
		}
	}
}

// List returns documents under a prefix, sorted by key.
func (s *Store) List(prefix string) []Doc {
	docKeySet := make(map[string]struct{})
	s.collectDocs(prefix, docKeySet)

	var entries []Doc
	for docKey := range docKeySet {
		meta := s.loadMeta(docKey)
		if meta != nil {
			entries = append(entries, Doc{
				Key:         docKey,
				Description: meta.Description,
				NumChunks:   meta.NumChunks,
				Stored:      meta.Stored,
			})
		} else {
			numChunks := 0
			for k := range s.kv.List(docKey) {
				if isChunkKey(k) {
					numChunks++
				}
			}
			entries = append(entries, Doc{Key: docKey, NumChunks: numChunks})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries
}

// ChunkText returns the text of chunk i of a document, or "" if unavailable.
func (s *Store) ChunkText(docKey string, i int) string {
	data, err := s.kv.Get(fmt.Sprintf("%s/chunk/%04d", docKey, i))
	if err != nil || len(data) == 0 {
		return ""
	}
	var ch struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(data, &ch) == nil {
		return ch.Text
	}
	return ""
}

// Delete removes a document and all its entries (chunks and metadata).
func (s *Store) Delete(docKey string) (int, error) {
	deleted := 0
	for _, key := range s.allKeys(docKey) {
		if err := s.kv.Del(key); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}
