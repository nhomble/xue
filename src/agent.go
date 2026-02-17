package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Agent defines how to build a command for running an AI agent.
type Agent interface {
	// BuildCommand creates an exec.Cmd for the given prompt, workspace settings, and repos.
	BuildCommand(prompt string, settings WorkspaceSettings, repos []string) *exec.Cmd
}

// ClaudeAgent invokes the Claude CLI.
type ClaudeAgent struct{}

func (c ClaudeAgent) BuildCommand(prompt string, settings WorkspaceSettings, repos []string) *exec.Cmd {
	args := []string{"-p", prompt}
	if settings.Model != "" {
		args = append(args, "--model", settings.Model)
	}
	if settings.MaxTokens > 0 {
		args = append(args, "--max-tokens", fmt.Sprintf("%d", settings.MaxTokens))
	}
	if settings.SystemPrompt != "" {
		args = append(args, "--system-prompt", settings.SystemPrompt)
	}
	for _, repo := range repos {
		args = append(args, "--add-dir", repo)
	}

	cmd := exec.Command("claude", args...)

	// Unset CLAUDECODE so claude doesn't refuse to run inside claude code
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			filtered = append(filtered, e)
		}
	}
	cmd.Env = filtered

	if len(repos) > 0 {
		cmd.Dir = repos[0]
	}

	return cmd
}
