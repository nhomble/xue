package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello", "hello"},
		{"CSI color", "\x1b[31mred\x1b[0m", "red"},
		{"CSI cursor", "\x1b[2Jhello", "hello"},
		{"CSI private mode", "\x1b[?25l", ""},
		{"OSC title", "\x1b]0;title\x07rest", "rest"},
		{"mixed", "\x1b[1mbold\x1b[0m plain", "bold plain"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI(tt.input)
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildTranscript(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.jsonl")

	f, _ := os.Create(logPath)
	entries := []LogEntry{
		{Ts: "2024-01-01T00:00:00Z", Stream: "out", Data: "hello "},
		{Ts: "2024-01-01T00:00:01Z", Stream: "out", Data: "\x1b[31mworld\x1b[0m"},
	}
	for _, e := range entries {
		line, _ := json.Marshal(e)
		f.Write(line)
		f.Write([]byte("\n"))
	}
	f.Close()

	transcript, err := buildTranscript(logPath)
	if err != nil {
		t.Fatalf("buildTranscript failed: %v", err)
	}

	if !strings.Contains(transcript, "hello") {
		t.Error("transcript should contain 'hello'")
	}
	if !strings.Contains(transcript, "world") {
		t.Error("transcript should contain 'world'")
	}
	if strings.Contains(transcript, "\x1b") {
		t.Error("transcript should not contain ANSI escapes")
	}
}

func TestBuildTranscriptMissingFile(t *testing.T) {
	_, err := buildTranscript("/nonexistent/path.jsonl")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
