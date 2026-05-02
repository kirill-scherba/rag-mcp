// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
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
	defaultLLMModel = "gemma3:4b" // "qwen2.5:1.5b"
	ollamaBaseURL   = "http://localhost:11434"
	generateTimeout = 120 * time.Second
)

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
func buildRAGPrompt(chunks []ragResult, question string) ([]OllamaChatMessage, error) {
	var contextParts []string
	for i, ch := range chunks {
		contextParts = append(contextParts, fmt.Sprintf(
			"--- Fragment %d ---\n%s", i+1, ch.Text))
	}
	context := strings.Join(contextParts, "\n\n")

	systemMsg := fmt.Sprintf(`You are a helpful AI assistant answering questions about the Cooksy project knowledge base.

Rules:
- Answer the question based ONLY on the context fragments provided below.
- If the context does not contain enough information, say so honestly.
- Do not make up or hallucinate information.
- Be concise but thorough.
- Use natural, fluent language.

Context:
%s`, context)

	messages := []OllamaChatMessage{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: question},
	}
	return messages, nil
}

// parseOllamaResponse handles both streaming (NDJSON) and non-streaming JSON
// responses from the Ollama /api/chat endpoint.
func parseOllamaResponse(data []byte) (string, error) {
	// Try parsing as single JSON object first (non-streaming response)
	var singleResp OllamaChatResponse
	if err := json.Unmarshal(data, &singleResp); err == nil {
		if singleResp.Message != nil {
			return strings.TrimSpace(singleResp.Message.Content), nil
		}
	}

	// Fallback: parse as NDJSON (streaming response with one JSON object per line)
	var answerParts []string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var chunk OllamaChatResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue // skip malformed lines
		}
		if chunk.Message != nil {
			answerParts = append(answerParts, chunk.Message.Content)
		}
		if chunk.Done {
			break
		}
	}
	if len(answerParts) > 0 {
		answer := strings.Join(answerParts, "")
		return strings.TrimSpace(answer), nil
	}

	return "", fmt.Errorf("failed to parse Ollama response (body: %s)", string(data))
}

// generateAnswer sends a chat request to Ollama and returns the generated answer.
func generateAnswer(messages []OllamaChatMessage) (string, error) {
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = ollamaBaseURL
	}

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = defaultLLMModel
	}

	// Some Ollama models always stream, so we handle both streaming and non-streaming.
	reqBody := OllamaChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   boolPtr(false),
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{Timeout: generateTimeout}
	resp, err := client.Post(baseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("Ollama chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama returned error %d: %s", resp.StatusCode, string(respBody))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read Ollama response: %w", err)
	}

	return parseOllamaResponse(data)
}