package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		printUsage()
		os.Exit(1)
	}

	launchTUI()
}

func launchTUI() {
	store, err := NewWorkspaceStore()
	if err != nil {
		fatal("failed to initialize workspace store: %v", err)
	}

	threads := NewThreadManager(store, ClaudeAgent{})
	threads.LoadAllThreads()
	defer threads.StopAll()

	if err := runTUI(store, threads); err != nil {
		fatal("TUI error: %v", err)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "xue — study your repos with AI\n\nUsage:\n  xue    Launch TUI\n")
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "xue: "+format+"\n", args...)
	os.Exit(1)
}
