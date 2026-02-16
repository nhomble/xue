package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerCreatesFile(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir, "test", "step1")
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	if logger.Path() == "" {
		t.Error("expected non-empty log path")
	}
	if _, err := os.Stat(logger.Path()); err != nil {
		t.Errorf("log file should exist: %v", err)
	}
}

func TestLoggerWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir, "test", "step1")
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	logger.Log("out", []byte("hello world"))
	logger.Log("in", []byte("user input"))
	logger.Close()

	data, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(lines))
	}

	var entry LogEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}
	if entry.Stream != "out" {
		t.Errorf("expected stream 'out', got %s", entry.Stream)
	}
	if entry.Data != "hello world" {
		t.Errorf("expected data 'hello world', got %s", entry.Data)
	}
	if entry.Ts == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestLoggerPath(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir, "prefix", "mystep")
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	path := logger.Path()
	if !strings.Contains(path, "prefix") {
		t.Errorf("path should contain prefix dir: %s", path)
	}
	if !strings.Contains(filepath.Base(path), "mystep") {
		t.Errorf("filename should contain step name: %s", path)
	}
}
