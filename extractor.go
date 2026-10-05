// Copyright 2026 Kirill Scherba. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Environment variables controlling file ingestion.
//
// EXTRACTOR_CMD: when set, every ingested file is passed to this command and
// its stdout is used as the document text. The command is tokenized like a
// shell word sequence (quotes supported, no pipes/redirection); the placeholder
// {file} is replaced with the file path, and when the placeholder is absent the
// path is appended as the last argument. Example:
//
//	EXTRACTOR_CMD='pdftotext {file} -'
//
// When EXTRACTOR_CMD is empty, the file is read as-is and treated as text
// (the original behaviour).
//
// EXTRACTOR_TIMEOUT: optional per-file timeout (Go duration, default 10m).
const (
	extractorCmdEnv     = "EXTRACTOR_CMD"
	extractorTimeoutEnv = "EXTRACTOR_TIMEOUT"
)

const defaultExtractorTimeout = 10 * time.Minute

// extractFileText turns a file into text. With EXTRACTOR_CMD set the command
// does the extraction; otherwise the file is read as text.
func extractFileText(ctx context.Context, path string) (string, error) {
	cmdTemplate := strings.TrimSpace(os.Getenv(extractorCmdEnv))
	if cmdTemplate == "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return runExtractor(ctx, cmdTemplate, path)
}

// runExtractor executes the configured extractor command for one file.
func runExtractor(ctx context.Context, cmdTemplate, path string) (string, error) {
	argv, err := tokenizeCommand(cmdTemplate)
	if err != nil {
		return "", err
	}
	if len(argv) == 0 {
		return "", fmt.Errorf("%s is empty", extractorCmdEnv)
	}

	replaced := false
	for i := range argv {
		if strings.Contains(argv[i], "{file}") {
			argv[i] = strings.ReplaceAll(argv[i], "{file}", path)
			replaced = true
		}
	}
	if !replaced {
		argv = append(argv, path)
	}

	timeout := defaultExtractorTimeout
	if v := strings.TrimSpace(os.Getenv(extractorTimeoutEnv)); v != "" {
		if d, perr := time.ParseDuration(v); perr == nil && d > 0 {
			timeout = d
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("extractor timed out after %s for %s", timeout, path)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("extractor %q failed for %s: %s", argv[0], path, msg)
	}
	return stdout.String(), nil
}

// tokenizeCommand splits a command string into arguments, honouring single and
// double quotes and backslash escapes. It deliberately supports no pipes or
// redirection — use a wrapper script for that.
func tokenizeCommand(s string) ([]string, error) {
	var (
		args    []string
		cur     strings.Builder
		started bool
		in1     bool // inside single quotes
		in2     bool // inside double quotes
		esc     bool
	)
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc, started = false, true
		case r == '\\' && !in1:
			esc, started = true, true
		case r == '\'' && !in2:
			in1, started = !in1, true
		case r == '"' && !in1:
			in2, started = !in2, true
		case (r == ' ' || r == '\t') && !in1 && !in2:
			if started {
				args = append(args, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if in1 || in2 {
		return nil, fmt.Errorf("unbalanced quotes in %s", extractorCmdEnv)
	}
	if started {
		args = append(args, cur.String())
	}
	return args, nil
}
