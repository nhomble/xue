package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// SessionResult holds the outcome of a PTY session.
type SessionResult struct {
	ExitCode int
	LogPath  string
	EndCmd   Command // which command ended the session (CommandNone if agent exited naturally)
}

func runPTY(command string, args []string, logger *Logger, stepName string) (SessionResult, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = os.Environ()

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return SessionResult{ExitCode: 1}, fmt.Errorf("failed to start PTY: %w", err)
	}
	defer ptmx.Close()

	// Handle SIGWINCH: resize PTY to match user's terminal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	// Set initial size
	sigCh <- syscall.SIGWINCH

	// Put user's terminal in raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return SessionResult{ExitCode: 1}, fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	startTime := time.Now()

	// Interceptor for slash commands
	interceptor := NewInterceptor(func(b []byte) {
		ptmx.Write(b)
	}, logger, stepName)

	// Channel to signal session end from interceptor
	cmdCh := make(chan Command, 1)

	// Goroutine: copy PTY stdout → user's stdout, logging output
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				os.Stdout.Write(buf[:n])
				if logger != nil {
					logger.Log("out", buf[:n])
				}
			}
			if err != nil {
				break
			}
		}
		close(done)
	}()

	// Goroutine: read user's stdin, intercept commands, forward the rest
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if logger != nil {
					logger.Log("in", buf[:n])
				}
				if cmd := interceptor.Feed(buf[:n]); cmd != CommandNone {
					if HandleCommand(cmd, logger, stepName, startTime) {
						cmdCh <- cmd
						return
					}
				}
			}
			if err != nil {
				break
			}
		}
	}()

	// Wait for either agent exit or slash command
	var endCmd Command
	select {
	case endCmd = <-cmdCh:
		// User issued /next or /end — kill the agent
		cmd.Process.Signal(syscall.SIGTERM)
		cmd.Wait()
	case <-done:
		// Agent exited on its own — wait is already done
	}

	// Wait for stdout to drain (if not already closed)
	<-done

	// Stop signal handling
	signal.Stop(sigCh)
	close(sigCh)

	logPath := ""
	if logger != nil {
		logPath = logger.Path()
	}

	return SessionResult{ExitCode: 0, LogPath: logPath, EndCmd: endCmd}, nil
}
