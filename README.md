# ccwrapper

A lightweight web UI that wraps the [Claude Code](https://docs.anthropic.com/en/docs/claude-code) CLI. It runs Claude in non-interactive `--print` mode, parses the streaming JSON output, and presents it in a clean, collapsible browser interface.

## How it works

Each time you submit a prompt, ccwrapper:

1. Appends your prompt to a **context file** (a plain-text transcript)
2. Launches `claude --print --output-format stream-json` with the full context as the prompt
3. Streams the JSON events to the browser over SSE
4. Appends Claude's response back to the context file

This simulates multi-turn conversation across separate single-shot CLI invocations. Claude state (history, todos, cache, etc.) is wiped before each run for isolation.

## UI

- **Prompt bar** at the top — type and hit Enter
- **Output tab** — each exchange is a collapsible "turn" showing the user prompt and assistant response. Thinking blocks and tool results start collapsed. Click to expand.
- **Context tab** — view and clear the accumulated transcript that gets sent as the prompt
- **System Prompt tab** — set a custom system prompt for all subsequent runs
- **System tab** — raw JSON of Claude's system init event (model, tools, session info, etc.)
- **Command tab** — the exact `claude` CLI invocation that was run, with all flags
- **Token totals** — running input/output token counts and cost in the tab bar
- **Answer Question** — click on a completed turn to load the assistant's text into the prompt for editing and re-sending
- Press **`s`** to toggle auto-scroll

## Building

Requires Go 1.25+. The `web/` directory is embedded into the binary.

```bash
# Linux
go build -o ccwrapper .

# macOS
GOOS=darwin GOARCH=arm64 go build -o ccwrapper-darwin-arm64 .
GOOS=darwin GOARCH=amd64 go build -o ccwrapper-darwin-amd64 .
```

## Running

```bash
# Start the server (requires `claude` CLI on PATH)
./ccwrapper
# Open http://localhost:8080

# Replay a saved stream-json file for testing
./ccwrapper --demo example.json
```

## Container

```bash
# Build the container image
./build-env.sh

# Launch an interactive shell in the container
./launch-env.sh
```

The Containerfile sets up Alpine with Go, the Claude CLI, and [beads_rust](https://github.com/Dicklesworthstone/beads_rust) for issue tracking.

## Claude CLI flags

ccwrapper launches Claude with these flags:

- `--print` — non-interactive single-shot mode
- `--output-format stream-json` — structured streaming output
- `--verbose` — include thinking blocks
- `--dangerously-skip-permissions` — no permission prompts
- `--disable-slash-commands` — no slash command processing
- `--no-session-persistence` — don't persist session state
- `--mcp-config ""` / `--strict-mcp-config` — disable MCP servers
- `--disallowed-tools` — disables tools not suited for headless operation

## Stack

- **Go** — HTTP server, CLI process management, JSON stream parsing
- **Vanilla JS** — no frameworks, single `app.js`
- **Pico CSS** — minimal dark-theme styling
- **SSE** — real-time event streaming to the browser
