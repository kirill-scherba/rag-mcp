// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// ragClient wraps an MCP stdio client connected to rag-mcp.
type ragClient struct {
	c *client.Client
}

var (
	// globalClient is the singleton MCP client shared by all subcommands.
	globalClient   *ragClient
	initClientOnce sync.Once
	initClientErr  error
)

// initClient discovers rag-mcp and initializes the global MCP client.
func initClient() error {
	initClientOnce.Do(func() {
		globalClient, initClientErr = newRagClient()
	})
	return initClientErr
}

// newRagClient discovers the rag-mcp binary and starts an MCP stdio client.
func newRagClient() (*ragClient, error) {
	ragPath := discoverRagMcp()
	if ragPath == "" {
		return nil, fmt.Errorf("rag-mcp binary not found; ensure it is in PATH, same directory, or GOPATH/bin")
	}

	// Build arguments for rag-mcp
	var args []string
	if dbPath != "" {
		args = append(args, "--db", dbPath)
	}
	if modelOverride != "" {
		args = append(args, "--model", modelOverride)
	}
	// Enable stderr token streaming so user sees live LLM tokens
	args = append(args, "--stream-stderr")

	mcpClient, err := client.NewStdioMCPClient(ragPath, nil, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to start MCP client: %w", err)
	}

	// Initialize MCP session
	ctx := context.Background()
	_, err = mcpClient.Initialize(ctx, mcp.InitializeRequest{})
	if err != nil {
		_ = mcpClient.Close()
		return nil, fmt.Errorf("MCP initialize failed: %w", err)
	}

	rc := &ragClient{
		c: mcpClient,
	}

	// Start stderr proxy in the background
	rc.proxyStderrWithThinking()

	return rc, nil
}

// discoverRagMcp searches for the rag-mcp binary in:
// 1. PATH
// 2. Same directory as this binary
// 3. GOPATH/bin
func discoverRagMcp() string {
	// 1. PATH
	if path, err := exec.LookPath("rag-mcp"); err == nil {
		return path
	}

	// 2. Same directory as rag-cli
	if exeDir := getExecutablePath(); exeDir != "" {
		candidate := filepath.Join(exeDir, "rag-mcp")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 3. GOPATH/bin
	if gopBin := goPathBin(); gopBin != "" {
		candidate := filepath.Join(gopBin, "rag-mcp")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

// callTool invokes a named tool with the given arguments and returns the text content.
func (r *ragClient) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}

	result, err := r.c.CallTool(ctx, req)
	if err != nil {
		return "", fmt.Errorf("tool %s call failed: %w", name, err)
	}

	var parts []string
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}

	return strings.Join(parts, ""), nil
}

// close shuts down the MCP client and waits for the server process.
func (r *ragClient) close() error {
	if r.c == nil {
		return nil
	}
	return r.c.Close()
}

// proxyStderrWithThinking proxies the server stderr to the user's stderr,
// showing a "Thinking..." spinner until the first LLM token arrives.
func (r *ragClient) proxyStderrWithThinking() {
	stderrReader, ok := client.GetStderr(r.c)
	if !ok || stderrReader == nil {
		return
	}

	go func() {
		_ = streamStderrWithMarker(stderrReader, os.Stderr)
	}()
}

// streamStderrWithMarker copies data from src to dst, buffering until the
// marker "---LLM---" is found. Once the marker is seen it prints a spinner,
// then flushes all remaining data immediately. Any trailing data is also
// flushed on io.EOF.
func streamStderrWithMarker(src io.Reader, dst io.Writer) error {
	var buf bytes.Buffer
	tokenSeen := false
	marker := []byte("---LLM---")
	tmp := make([]byte, 1024)

	for {
		n, err := src.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
			for {
				data := buf.Bytes()
				if len(data) == 0 {
					break
				}
				if !tokenSeen {
					idx := bytes.Index(data, marker)
					if idx == -1 {
						// Marker not yet seen; keep buffered data for next read
						break
					}
					tokenSeen = true
					if idx > 0 {
						// Print any prefix data before the marker
						dst.Write(data[:idx])
					}
					fmt.Fprintln(dst, "\r✅ Thinking...")
					buf.Next(idx + len(marker))
					continue
				}
				// After marker: flush all buffered data immediately
				dst.Write(data)
				buf.Reset()
				break
			}
		}
		if err != nil {
			if err != io.EOF {
				_ = err
			}
			// Flush any remaining buffered data before exit
			if buf.Len() > 0 {
				dst.Write(buf.Bytes())
				buf.Reset()
			}
			return nil
		}
	}
}
