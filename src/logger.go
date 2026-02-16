package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LogEntry struct {
	Ts     string `json:"ts"`
	Stream string `json:"stream"`
	Data   string `json:"data"`
}

type Logger struct {
	mu      sync.Mutex
	writer  *bufio.Writer
	file    *os.File
	logPath string
	start   time.Time
}

func NewLogger(outputDir, prefix, stepName string) (*Logger, error) {
	dir := filepath.Join(outputDir, prefix)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	ts := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s.jsonl", stepName, ts)
	logPath := filepath.Join(dir, filename)

	f, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	return &Logger{
		writer:  bufio.NewWriter(f),
		file:    f,
		logPath: logPath,
		start:   time.Now(),
	}, nil
}

func (l *Logger) Log(stream string, data []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := LogEntry{
		Ts:     time.Now().UTC().Format(time.RFC3339Nano),
		Stream: stream,
		Data:   string(data),
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	l.writer.Write(line)
	l.writer.WriteByte('\n')
}

func (l *Logger) Path() string {
	return l.logPath
}

func (l *Logger) Duration() time.Duration {
	return time.Since(l.start)
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writer.Flush()
	return l.file.Close()
}
