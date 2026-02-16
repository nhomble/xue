package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// CSI sequences: ESC [ ... letter (including ? for private modes)
	ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)
	// OSC sequences: ESC ] ... (terminated by BEL or ST)
	ansiOSC = regexp.MustCompile(`\x1b\].*?(\x07|\x1b\\)`)
	// Other single-char escapes
	ansiOther = regexp.MustCompile(`\x1b[()][0-9A-Za-z]`)
)

func stripANSI(s string) string {
	s = ansiCSI.ReplaceAllString(s, "")
	s = ansiOSC.ReplaceAllString(s, "")
	s = ansiOther.ReplaceAllString(s, "")
	return s
}

// buildTranscript reads a JSONL log file and produces a clean text transcript.
func buildTranscript(logPath string) (string, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var sb strings.Builder
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer

	for scanner.Scan() {
		var entry LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		sb.WriteString(stripANSI(entry.Data))
	}

	return sb.String(), scanner.Err()
}

// summarize pipes the transcript through the local claude CLI in print mode.
func summarize(transcript, model, prompt string) (string, error) {
	if prompt == "" {
		prompt = defaultSummaryPrompt
	}

	args := []string{
		"-p",
		"--system-prompt", prompt,
		"--no-session-persistence",
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	cmd := exec.Command("claude", args...)
	cmd.Stdin = strings.NewReader(transcript)

	// Unset CLAUDECODE so claude doesn't refuse to run inside our session
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			filtered = append(filtered, e)
		}
	}
	cmd.Env = filtered

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("claude CLI failed: %w: %s", err, string(out))
	}

	return strings.TrimSpace(string(out)), nil
}

// summarizeSession reads the log, builds transcript, calls claude CLI, writes summary file.
// Returns the summary text. On error, logs to stderr and returns empty string.
func summarizeSession(logPath, stepName, model, prompt string) string {
	transcript, err := buildTranscript(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[xue] Failed to read log for summary: %v\n", err)
		return ""
	}

	if strings.TrimSpace(transcript) == "" {
		fmt.Fprintf(os.Stderr, "[xue] Empty transcript, skipping summary\n")
		return ""
	}

	fmt.Fprintf(os.Stderr, "[xue] Generating summary...\n")

	summary, err := summarize(transcript, model, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[xue] Summarization failed: %v\n", err)
		return ""
	}

	// Write summary file alongside the log
	dir := filepath.Dir(logPath)
	summaryPath := filepath.Join(dir, stepName+"-summary.md")
	if err := os.WriteFile(summaryPath, []byte(summary+"\n"), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[xue] Failed to write summary: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "[xue] Summary written to %s\n", summaryPath)
	}

	return summary
}
