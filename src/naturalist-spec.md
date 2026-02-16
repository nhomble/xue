# naturalist

A transparent PTY proxy for observing and chaining AI coding agent sessions.

## What It Does

`naturalist` sits between you and an AI coding agent (Claude Code, Cursor agent, etc.), transparently passing all stdin/stdout through while intercepting a small set of slash commands. It logs sessions, and when you're done with one agent, it summarizes the conversation and hands context off to the next agent session automatically.

Think of it as a field researcher observing specimens — the agents don't know they're being watched, and naturalist takes notes so the next observer has context.

## Core Concepts

**Session**: A single PTY-proxied agent invocation. Everything the agent prints and everything you type flows through unmodified. naturalist captures it all in a structured log.

**Transition**: When you end a session (via `/next` or `/end`), naturalist summarizes the session transcript via an LLM call and either injects that summary into the next session's context or writes it to a final output file.

**Pipeline**: An optional ordered sequence of sessions defined in a config file, each with its own agent command and optional system prompt injection.

## Architecture

```
┌─────────────┐     stdin      ┌─────────────────┐     stdin      ┌───────────────┐
│             │ ──────────────▶│                 │──────────────▶│               │
│  User TTY   │                │   naturalist    │                │  Agent PTY    │
│             │ ◀──────────────│   (intercepts   │◀──────────────│  (claude,     │
│             │     stdout     │    /commands)   │     stdout     │   cursor,etc) │
└─────────────┘                └─────────────────┘                └───────────────┘
                                       │
                                       │ writes
                                       ▼
                               ┌─────────────────┐
                               │  Session Log    │
                               │  (JSONL file)   │
                               └─────────────────┘
```

## Usage

### Ad-hoc single session

```bash
# Wrap any agent command
naturalist -- claude

# Wrap with arguments
naturalist -- cursor-agent -p "analyze this codebase"
```

### Pipeline mode

```bash
# Run a predefined pipeline
naturalist run pipeline.yaml
```

### Pipeline config format

```yaml
# pipeline.yaml
name: "codebase-review"
summarizer:
  model: "claude-sonnet-4-20250514"
  # Optional: override the default summarizer prompt
  prompt: "Summarize focusing on architectural decisions and open questions."

steps:
  - name: "analyze"
    command: "claude"
    # Optional: text prepended to the agent's stdin on startup
    inject: "Analyze this repository structure and identify the main architectural patterns."

  - name: "security-review"
    command: "claude"
    inject: "Given the previous analysis, review this codebase for security concerns."
    # The summary from the previous step is automatically prepended before inject

  - name: "plan"
    command: "claude"
    inject: "Based on the analysis and security review, create an implementation plan for improvements."
```

## Slash Commands

These are intercepted by naturalist and never forwarded to the agent.

| Command | Description |
|---------|-------------|
| `/next` | End current session, summarize, start next step (pipeline mode) or prompt for new command (ad-hoc mode) |
| `/end` | End current session, summarize, and exit naturalist entirely |
| `/log` | Print the path to the current session's log file |
| `/status` | Show current step name, session duration, and log size |

Everything else — including lines that start with `/` but aren't recognized — passes through to the agent.

## Session Logging

Each session produces a JSONL file in the output directory. One line per chunk:

```jsonl
{"ts":"2026-02-14T22:01:03.412Z","stream":"out","data":"Claude Code v1.2.3\n"}
{"ts":"2026-02-14T22:01:05.100Z","stream":"in","data":"analyze src/\n"}
{"ts":"2026-02-14T22:01:07.882Z","stream":"out","data":"I'll look at the source directory...\n"}
```

**Fields:**

- `ts` — ISO 8601 timestamp
- `stream` — `"in"` for user input (stdin), `"out"` for agent output (stdout)
- `data` — raw string content. May contain ANSI escape codes from TUI agents.

**File naming:** `{output_dir}/{pipeline_name}/{step_name}-{timestamp}.jsonl`

For ad-hoc sessions: `{output_dir}/adhoc/session-{timestamp}.jsonl`

Default output directory: `.naturalist/` in the current working directory.

## Context Transitions

When a session ends (via `/next` or `/end`), naturalist:

1. Reads the session's JSONL log
2. Strips ANSI escape codes from the raw data to produce a clean text transcript
3. Sends the transcript to the configured summarizer model with this default prompt:

```
Summarize this AI agent session in under 300 words.
Focus on: decisions made, questions raised, current state of the work, and any unresolved items.
Do not include greetings, pleasantries, or meta-commentary about the conversation itself.
```

4. Writes the summary to `{step_name}-summary.md` alongside the log
5. If there's a next step, prepends the summary to that step's `inject` text and writes it to the new agent's stdin after spawn

## Technical Requirements

### Language and Build

- **Go** (1.22+)
- Single binary output, no runtime dependencies
- Use `creack/pty` for PTY management

### PTY Proxy

- Spawn the agent command inside a PTY via `creack/pty`
- Set up two goroutines:
  - **stdin reader**: reads from os.Stdin, checks for slash commands, forwards everything else to the PTY's stdin, logs `"in"` entries
  - **stdout reader**: reads from the PTY's stdout, writes to os.Stdout, logs `"out"` entries
- Handle terminal resize signals (SIGWINCH) and forward to the PTY
- On agent process exit, flush logs and trigger the transition flow

### Slash Command Interception

- The stdin reader checks each line for recognized `/commands` **before** forwarding
- Matching is exact: the line must start with the command and contain nothing else (or whitespace after)
- Unrecognized `/` lines pass through — don't break agent workflows that use `/` syntax

### LLM Summarizer

- HTTP POST to `https://api.anthropic.com/v1/messages`
- Reads API key from `ANTHROPIC_API_KEY` environment variable
- Model defaults to `claude-sonnet-4-20250514`, overridable in pipeline config
- Max tokens: 1024 for summary responses
- No SDK dependency — raw HTTP with `net/http`

### ANSI Stripping

- Before sending transcript to the summarizer, strip all ANSI escape sequences
- Simple regex: `\x1b\[[0-9;]*[a-zA-Z]` covers CSI sequences
- Also strip `\x1b\]` OSC sequences (terminated by `\x07` or `\x1b\\`)

### Signal Handling

- Forward SIGWINCH to the PTY for terminal resize
- SIGINT/SIGTERM: forward to the agent process, wait for exit, then flush logs and summarize
- Don't trap SIGINT aggressively — let the agent handle it first (e.g., Claude Code's graceful shutdown)

## Project Structure

```
naturalist/
├── main.go              # CLI entry point, arg parsing, command dispatch
├── pty.go               # PTY spawn, resize handling, stdin/stdout proxy goroutines
├── intercept.go         # Slash command detection and dispatch
├── logger.go            # JSONL session logger (append-only, buffered writes)
├── summarizer.go        # ANSI stripping, transcript assembly, LLM API call
├── pipeline.go          # YAML config parsing, step sequencing, context injection
├── config.go            # Config types, defaults, validation
├── go.mod
├── go.sum
└── README.md
```

## CLI Interface

```
naturalist [flags] -- <command> [args...]    # ad-hoc mode
naturalist run <pipeline.yaml> [flags]       # pipeline mode

Flags:
  -o, --output-dir string    Output directory for logs and summaries (default ".naturalist")
  -v, --verbose              Print naturalist status messages to stderr
  --no-summary               Skip LLM summarization on session end
  --dry-run                  (pipeline mode) Print the steps without executing
```

## Error Handling

- If the agent command is not found, exit with a clear error message
- If the LLM summarizer call fails (network error, auth error, rate limit), log the error to stderr, skip summarization, and continue to the next step. Don't block the pipeline on a failed summary.
- If a pipeline step's agent exits with a non-zero code, log a warning but continue to the next step. The user can check the log to understand what happened.
- If the JSONL log file can't be created (permissions, disk full), print error to stderr and continue running without logging — the proxy function is more important than the logging function.

## Dependencies

- `github.com/creack/pty` — PTY management
- `gopkg.in/yaml.v3` — pipeline config parsing
- Standard library only for everything else (no Cobra, no Viper — keep it minimal)

## Out of Scope (for v1)

- Parallel agent sessions
- Interactive TUI for naturalist itself (no bubbletea, just slash commands)
- Persistent session database (JSONL files are the database)
- Remote agent support (local PTY only)
- Windows support (PTY semantics differ — Unix-only for now)
