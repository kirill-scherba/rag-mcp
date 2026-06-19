// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Default LLM models and Ollama settings.
const (
	defaultLLMModel = "phi4-mini" // "deepseek-v4-flash:cloud" // "gemma3:4b" // "qwen2.5:1.5b"
	ollamaBaseURL   = "http://localhost:11434"
	generateTimeout = 120 * time.Second
)

// ollamaClient is a reusable HTTP client with keep-alive transport.
var ollamaClient = &http.Client{
	Timeout: generateTimeout,
	Transport: &http.Transport{
		MaxIdleConns:    5,
		IdleConnTimeout: 90 * time.Second,
	},
}

// ollamaModelOverride overrides the LLM model when set via --model flag.
var ollamaModelOverride string

// OllamaChatMessage represents a message in the chat API.
type OllamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OllamaChatRequest is the request to Ollama /api/chat.
type OllamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []OllamaChatMessage `json:"messages"`
	Stream   *bool               `json:"stream,omitempty"`
}

// OllamaChatResponse is the response from Ollama /api/chat.
type OllamaChatResponse struct {
	Message *OllamaChatMessage `json:"message,omitempty"`
	Done    bool               `json:"done"`
}

// boolPtr returns a pointer to the given boolean value.
func boolPtr(b bool) *bool { return &b }

// buildRAGPrompt constructs the LLM chat messages with system instruction,
// context chunks and the user's question.
// style can be "strict" (copy-paste of exact signatures) or "creative" (free-form answer).
func buildRAGPrompt(chunks []ragResult, question, style string) ([]OllamaChatMessage, error) {
	var contextParts []string
	for i, ch := range chunks {
		contextParts = append(contextParts, fmt.Sprintf(
			"--- Fragment %d ---\n%s", i+1, ch.Text))
	}
	context := strings.Join(contextParts, "\n\n")

	var systemMsg string
	if style == "creative" {
		systemMsg = fmt.Sprintf(`You are a knowledgeable and helpful assistant. Answer the user's question using the provided context fragments. You may explain, analyze, connect ideas, and draw conclusions. Be conversational and thorough.

RULES:
1. Use the context fragments as your primary source of information.
2. If the context contains relevant information, explain it in your own words.
3. If the context does not contain the answer, say so clearly, then provide your best answer based on general knowledge.
4. You MAY add explanations, examples, and natural language.
5. You MAY draw inferences and connect multiple fragments together.

FRAGMENTS:
%s

Now answer the question using the fragments above.`, context)
	} else {
		// strict mode (default): exact copy-paste, no extra text
		systemMsg = fmt.Sprintf(`You are a strict copy-paste assistant. Your ONLY job: find relevant function signatures in the fragments below and output them EXACTLY as written.

RULES:
1. Look at the fragments. Find lines that contain a Go function declaration matching the question.
2. Output ONLY the exact function signature line(s) from the fragments. Nothing else.
3. If the signature is split across multiple fragment lines, output them exactly as they appear.
4. If you cannot find any matching function declaration in the fragments, output only: "Not found."
5. DO NOT add explanations, descriptions, inferences, or natural language.
6. DO NOT generate or complete any code not present verbatim in the fragments.

Example of CORRECT output:
func Insert[T any](db *sql.DB, rows ...T) (err error)

Example of INCORRECT output (DO NOT DO THIS):
"The Insert function accepts a database connection..."
or
"Based on Fragment 2, the function..."
or any text that is not the exact function signature.

FRAGMENTS:
%s

Now output the exact function signature(s) matching the question.`, context)
	}

	messages := []OllamaChatMessage{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: question},
	}
	return messages, nil
}

// TokenProgressFn is called with each token as it arrives from the LLM stream.
type TokenProgressFn func(token string)

// GenerateAnswerOptions controls answer streaming side effects.
type GenerateAnswerOptions struct {
	ProgressFn     TokenProgressFn
	StreamToStderr bool
}

// generateAnswerStream sends a streaming chat request to Ollama, calls
// progressFn for each token as it arrives, and returns the full answer.
// If progressFn is nil, tokens are accumulated silently.
// Tokens are also written to stderr for legacy CLI callers.
func generateAnswerStream(messages []OllamaChatMessage, progressFn TokenProgressFn) (string, error) {
	return generateAnswerStreamWithOptions(messages, GenerateAnswerOptions{
		ProgressFn:     progressFn,
		StreamToStderr: true,
	})
}

// generateAnswerStreamWithOptions sends a streaming chat request to Ollama,
// reports token progress, and returns the full answer.
func generateAnswerStreamWithOptions(messages []OllamaChatMessage, opts GenerateAnswerOptions) (string, error) {
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = ollamaBaseURL
	}

	model := ollamaModelOverride
	if model == "" {
		model = os.Getenv("LLM_MODEL")
	}
	if model == "" {
		model = defaultLLMModel
	}

	// Enable streaming so tokens arrive as they are generated
	reqBody := OllamaChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   boolPtr(true),
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := ollamaClient.Post(baseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("Ollama chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama returned error %d: %s", resp.StatusCode, string(respBody))
	}

	// Stream NDJSON response line by line.
	scanner := bufio.NewScanner(resp.Body)
	var answer strings.Builder
	streamStarted := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var chunk OllamaChatResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue // skip malformed lines
		}

		if chunk.Message != nil {
			token := chunk.Message.Content
			if !streamStarted {
				streamStarted = true
				if opts.StreamToStderr {
					fmt.Fprintf(os.Stderr, "---LLM---")
				}
			}
			if opts.ProgressFn != nil {
				opts.ProgressFn(token)
			}
			// Stream token to stderr so user sees live answer in terminal
			// (when not using progress-notification streaming)
			if opts.StreamToStderr {
				fmt.Fprintf(os.Stderr, "%s", token)
			}
			answer.WriteString(token)
		}

		if chunk.Done {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading Ollama stream: %w", err)
	}

	return strings.TrimSpace(answer.String()), nil
}
