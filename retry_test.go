// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestIsEmbedderNotReady verifies the error matcher.
func TestIsEmbedderNotReady(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"match", errors.New("embedder is not ready"), true},
		{"match substring", errors.New("the embedder is not ready yet"), true},
		{"non-match", errors.New("connection refused"), false},
		{"nil error", nil, false},
		{"empty error", errors.New(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmbedderNotReady(tt.err); got != tt.want {
				t.Errorf("isEmbedderNotReady(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestWithEmbedderRetrySuccessFirst verifies immediate success.
func TestWithEmbedderRetrySuccessFirst(t *testing.T) {
	result, err := withEmbedderRetry(context.Background(), func() (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result != 42 {
		t.Errorf("expected result 42, got %d", result)
	}
}

// TestWithEmbedderRetryThenSuccess verifies retry then success.
func TestWithEmbedderRetryThenSuccess(t *testing.T) {
	callCount := 0
	result, err := withEmbedderRetry(context.Background(), func() (string, error) {
		callCount++
		if callCount < 3 {
			return "", errors.New("embedder is not ready")
		}
		return "success", nil
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result != "success" {
		t.Errorf("expected result 'success', got %q", result)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

// TestWithEmbedderRetryExhausted verifies exhausted retries.
func TestWithEmbedderRetryExhausted(t *testing.T) {
	callCount := 0
	_, err := withEmbedderRetry(context.Background(), func() (int, error) {
		callCount++
		return 0, errors.New("embedder is not ready")
	})
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	if callCount != embedderRetryAttempts {
		t.Errorf("expected %d calls, got %d", embedderRetryAttempts, callCount)
	}
	if !isEmbedderNotReady(err) {
		t.Errorf("expected error to be embedder-not-ready: %v", err)
	}
}

// TestWithEmbedderRetryNonRetryable verifies immediate failure for non-retryable errors.
func TestWithEmbedderRetryNonRetryable(t *testing.T) {
	callCount := 0
	_, err := withEmbedderRetry(context.Background(), func() (int, error) {
		callCount++
		return 0, errors.New("connection refused")
	})
	if err == nil {
		t.Fatal("expected error for non-retryable")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call for non-retryable error, got %d", callCount)
	}
}

// TestWithEmbedderRetryContextCancel verifies context cancellation.
func TestWithEmbedderRetryContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	callCount := 0
	_, err := withEmbedderRetry(ctx, func() (int, error) {
		callCount++
		return 0, errors.New("embedder is not ready")
	})
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	// Should stop after first attempt when context is already canceled
	if callCount != 1 {
		t.Errorf("expected 1 call when context already canceled, got %d", callCount)
	}
}

// TestEmbedderRetryConstants verifies retry constants are reasonable.
func TestEmbedderRetryConstants(t *testing.T) {
	// 40 attempts × 250ms = 10 seconds max wait
	maxWait := time.Duration(embedderRetryAttempts) * embedderRetryDelay
	expected := 10 * time.Second
	if maxWait != expected {
		t.Errorf("max retry wait = %v, want %v", maxWait, expected)
	}
}