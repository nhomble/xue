package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func loadPipeline(path string) (*PipelineConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read pipeline config: %w", err)
	}

	var cfg PipelineConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse pipeline config: %w", err)
	}

	if cfg.Name == "" {
		return nil, fmt.Errorf("pipeline config missing 'name'")
	}
	if len(cfg.Steps) == 0 {
		return nil, fmt.Errorf("pipeline config has no steps")
	}
	for i, step := range cfg.Steps {
		if step.Command == "" {
			return nil, fmt.Errorf("step %d missing 'command'", i)
		}
		if step.Name == "" {
			cfg.Steps[i].Name = fmt.Sprintf("step-%d", i+1)
		}
	}

	return &cfg, nil
}

func runPipeline(cfg *PipelineConfig, globalCfg *Config) error {
	if globalCfg.DryRun {
		fmt.Fprintf(os.Stderr, "Pipeline: %s\n", cfg.Name)
		for i, step := range cfg.Steps {
			fmt.Fprintf(os.Stderr, "  Step %d: %s (command: %s)\n", i+1, step.Name, step.Command)
			if step.Inject != "" {
				fmt.Fprintf(os.Stderr, "    inject: %s\n", truncate(step.Inject, 80))
			}
		}
		return nil
	}

	model := cfg.Summarizer.Model
	prompt := cfg.Summarizer.Prompt
	var prevSummary string

	for i, step := range cfg.Steps {
		fmt.Fprintf(os.Stderr, "\n[xue] === Step %d/%d: %s ===\n", i+1, len(cfg.Steps), step.Name)

		logger, err := NewLogger(globalCfg.OutputDir, cfg.Name, step.Name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[xue] Warning: logging disabled for step %s: %v\n", step.Name, err)
		}

		// Build inject text: previous summary + step's inject
		var inject string
		if prevSummary != "" {
			inject = "## Context from previous step\n\n" + prevSummary + "\n\n"
		}
		if step.Inject != "" {
			inject += step.Inject
		}

		// Parse command (split on spaces for simple cases)
		cmdParts := strings.Fields(step.Command)
		command := cmdParts[0]
		cmdArgs := cmdParts[1:]

		// TODO: inject text into PTY stdin after spawn
		// For now, we run the session and the inject will be handled
		// by writing to the PTY after a brief delay
		_ = inject

		result, err := runPTY(command, cmdArgs, logger, step.Name)
		if logger != nil {
			logger.Close()
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "[xue] Step %s failed: %v\n", step.Name, err)
		}

		// Summarize unless disabled
		if !globalCfg.NoSummary && result.LogPath != "" {
			prevSummary = summarizeSession(result.LogPath, step.Name, model, prompt)
		}

		// If user issued /end, stop the pipeline
		if result.EndCmd == CommandEnd {
			fmt.Fprintf(os.Stderr, "[xue] Pipeline stopped by /end\n")
			break
		}
	}

	fmt.Fprintf(os.Stderr, "\n[xue] Pipeline complete\n")
	return nil
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// injectText writes text to the PTY stdin after a short delay.
// Used for context injection in pipeline mode.
func injectText(ptmx *os.File, text string) {
	if text == "" {
		return
	}
	time.Sleep(500 * time.Millisecond)
	ptmx.WriteString(text + "\n")
}
