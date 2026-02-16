package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Command represents the result of intercepting a slash command.
type Command int

const (
	CommandNone Command = iota
	CommandNext
	CommandEnd
	CommandLog
	CommandStatus
)

var knownCommands = map[string]Command{
	"/next":   CommandNext,
	"/end":    CommandEnd,
	"/log":    CommandLog,
	"/status": CommandStatus,
}

// knownPrefixes is the set of all prefixes of known commands (including "/").
// Used to decide whether to hold keystrokes during input buffering.
var knownPrefixes []string

func init() {
	seen := map[string]bool{}
	for cmd := range knownCommands {
		for i := 1; i <= len(cmd); i++ {
			p := cmd[:i]
			if !seen[p] {
				seen[p] = true
				knownPrefixes = append(knownPrefixes, p)
			}
		}
	}
}

func isKnownPrefix(s string) bool {
	for _, p := range knownPrefixes {
		if p == s {
			return true
		}
	}
	return false
}

// Interceptor buffers stdin in raw mode to detect slash commands.
// It holds keystrokes when the buffer could be a known command prefix,
// and flushes them to the PTY if it turns out not to be a command.
type Interceptor struct {
	buf       []byte
	holding   bool // true when buffer might be a command
	ptmxWrite func([]byte) // write to PTY stdin
	logger    *Logger
	stepName  string
	startTime time.Time
}

func NewInterceptor(ptmxWrite func([]byte), logger *Logger, stepName string) *Interceptor {
	return &Interceptor{
		ptmxWrite: ptmxWrite,
		logger:    logger,
		stepName:  stepName,
		startTime: time.Now(),
	}
}

// Feed processes raw bytes from stdin. Returns a Command if one was detected.
func (ic *Interceptor) Feed(data []byte) Command {
	for _, b := range data {
		if cmd := ic.feedByte(b); cmd != CommandNone {
			return cmd
		}
	}
	return CommandNone
}

func (ic *Interceptor) feedByte(b byte) Command {
	// Enter/CR: check if buffer is a complete command
	if b == '\r' || b == '\n' {
		if ic.holding {
			line := strings.TrimSpace(string(ic.buf))
			if cmd, ok := knownCommands[line]; ok {
				ic.buf = ic.buf[:0]
				ic.holding = false
				return cmd
			}
			// Not a command — flush held bytes + the enter key
			ic.ptmxWrite(ic.buf)
			ic.buf = ic.buf[:0]
			ic.holding = false
		}
		ic.ptmxWrite([]byte{b})
		return CommandNone
	}

	// Ctrl-C or other control chars: flush buffer and forward
	if b < 0x20 && b != '\t' {
		if ic.holding {
			ic.ptmxWrite(ic.buf)
			ic.buf = ic.buf[:0]
			ic.holding = false
		}
		ic.ptmxWrite([]byte{b})
		return CommandNone
	}

	// Backspace (0x7f): remove last byte from buffer if holding
	if b == 0x7f {
		if ic.holding && len(ic.buf) > 0 {
			ic.buf = ic.buf[:len(ic.buf)-1]
			if len(ic.buf) == 0 {
				ic.holding = false
			}
			// Forward backspace to PTY so the terminal reflects it
			ic.ptmxWrite([]byte{b})
			return CommandNone
		}
		ic.ptmxWrite([]byte{b})
		return CommandNone
	}

	// Build up the buffer
	ic.buf = append(ic.buf, b)
	candidate := string(ic.buf)

	if !ic.holding {
		// Start holding if buffer starts with "/"
		if candidate == "/" {
			ic.holding = true
			return CommandNone
		}
		// Not a potential command — forward immediately
		ic.ptmxWrite(ic.buf)
		ic.buf = ic.buf[:0]
		return CommandNone
	}

	// We're holding — check if still a valid prefix
	if isKnownPrefix(candidate) {
		return CommandNone // keep holding
	}

	// No longer a valid prefix — flush everything
	ic.ptmxWrite(ic.buf)
	ic.buf = ic.buf[:0]
	ic.holding = false
	return CommandNone
}

// HandleCommand executes a detected slash command. Returns true if xue should exit.
func HandleCommand(cmd Command, logger *Logger, stepName string, startTime time.Time) (shouldEnd bool) {
	switch cmd {
	case CommandLog:
		if logger != nil {
			fmt.Fprintf(os.Stderr, "\r\n[xue] Log: %s\r\n", logger.Path())
		} else {
			fmt.Fprintf(os.Stderr, "\r\n[xue] Logging disabled\r\n")
		}
	case CommandStatus:
		duration := time.Since(startTime).Truncate(time.Second)
		logSize := int64(0)
		if logger != nil {
			if info, err := os.Stat(logger.Path()); err == nil {
				logSize = info.Size()
			}
		}
		fmt.Fprintf(os.Stderr, "\r\n[xue] Step: %s | Duration: %s | Log size: %d bytes\r\n", stepName, duration, logSize)
	case CommandNext:
		fmt.Fprintf(os.Stderr, "\r\n[xue] Ending session (next)...\r\n")
		return true
	case CommandEnd:
		fmt.Fprintf(os.Stderr, "\r\n[xue] Ending session...\r\n")
		return true
	}
	return false
}
