package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	cfg := &Config{
		OutputDir: defaultOutputDir,
	}

	args := os.Args[1:]

	// Parse flags before -- or subcommand
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--output-dir":
			if i+1 >= len(args) {
				fatal("--output-dir requires a value")
			}
			i++
			cfg.OutputDir = args[i]
		case "-v", "--verbose":
			cfg.Verbose = true
		case "--no-summary":
			cfg.NoSummary = true
		case "--dry-run":
			cfg.DryRun = true
		case "--":
			positional = append(positional, args[i:]...)
			i = len(args)
		default:
			if strings.HasPrefix(args[i], "-") {
				fatal("unknown flag: %s", args[i])
			}
			positional = append(positional, args[i:]...)
			i = len(args)
		}
	}

	// No args → launch TUI dashboard
	if len(positional) == 0 {
		launchTUI()
		return
	}

	// Dispatch: "run" subcommand or "--" ad-hoc mode
	if positional[0] == "run" {
		runPipelineMode(positional[1:], cfg)
	} else if positional[0] == "--" {
		runAdHocMode(positional[1:], cfg)
	} else {
		printUsage()
		os.Exit(1)
	}
}

func launchTUI() {
	store, err := NewWorkspaceStore()
	if err != nil {
		fatal("failed to initialize workspace store: %v", err)
	}

	threads := NewThreadManager(store)
	threads.LoadAllThreads()
	defer threads.StopAll()

	if err := runTUI(store, threads); err != nil {
		fatal("TUI error: %v", err)
	}
}

func runAdHocMode(args []string, cfg *Config) {
	if len(args) == 0 {
		fatal("no command specified after --")
	}

	command := args[0]
	cmdArgs := args[1:]

	logger, err := NewLogger(cfg.OutputDir, "adhoc", "session")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[xue] Warning: logging disabled: %v\n", err)
	}

	result, runErr := runPTY(command, cmdArgs, logger, "session")
	if logger != nil {
		logger.Close()
	}

	if !cfg.NoSummary && result.LogPath != "" && (result.EndCmd == CommandEnd || result.EndCmd == CommandNext) {
		summarizeSession(result.LogPath, "session", "", "")
	}

	if runErr != nil {
		fmt.Fprintf(os.Stderr, "xue: %v\n", runErr)
		os.Exit(1)
	}
	os.Exit(result.ExitCode)
}

func runPipelineMode(args []string, cfg *Config) {
	if len(args) == 0 {
		fatal("usage: xue run <pipeline.yaml>")
	}

	pipelinePath := args[0]
	pipeline, err := loadPipeline(pipelinePath)
	if err != nil {
		fatal("%v", err)
	}

	if err := runPipeline(pipeline, cfg); err != nil {
		fatal("%v", err)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `xue - workspace orchestrator for AI agent sessions

Usage:
  xue                                    Launch TUI dashboard
  xue [flags] -- <command> [args...]     Ad-hoc mode
  xue [flags] run <pipeline.yaml>        Pipeline mode

Flags:
  -o, --output-dir string    Output directory for logs (default ".xue")
  -v, --verbose              Print status messages to stderr
  --no-summary               Skip LLM summarization on session end
  --dry-run                  (pipeline mode) Print steps without executing
`)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "xue: "+format+"\n", args...)
	os.Exit(1)
}
