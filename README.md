# ccwrapper

A web UI that wraps the [Claude Code](https://docs.anthropic.com/en/docs/claude-code) CLI around a structured **Plan → Refine → Execute** workflow. Each Claude invocation is a single-shot `--print` call with no internal state carried between runs — all project state lives in [beads_rust](https://github.com/Dicklesworthstone/beads_rust) (`br`) issues.

## How it works

ccwrapper drives Claude Code through three phases:

1. **Plan** — You describe your project in the Plan tab. ccwrapper appends your text to `prompts/plan.md` and sends it to Claude. Claude breaks the work into epics and tasks in `br`.

2. **Refine** — Click "Send Refine" to send `prompts/refine.md` verbatim. Claude reads `br` and asks clarifying questions. You answer via the "Answer Question" button on completed turns, which populates an inline editor in the Refine tab. Repeat until the plan is solid.

3. **Execute** — Click "Send Execute" to send `prompts/execute.md` verbatim. Claude runs `br ready` to find the next epic, implements it, updates `br`, and commits. Click Execute again for the next epic.

Before each run, ccwrapper wipes Claude's internal state (`~/.claude/`, `~/.cache/claude/`, and non-credential fields in `~/.claude.json`) so every invocation starts clean. The only continuity between runs is what's in `br` and the codebase itself.

## UI

- **Plan** — Textarea editor. Your text is appended to the `plan.md` template and sent to Claude. Shows a hint about this behavior.
- **Output** — Collapsible turns showing user prompt and assistant response. Thinking blocks and tool results start collapsed. When a turn finishes it auto-collapses and shows an "Answer Question" button that navigates to the Refine tab with the assistant's text pre-loaded for editing.
- **Refine** — Sends `refine.md` as the prompt. Shows current `br list` and the refine prompt preview. When answering questions, an inline editor and "Send Answers" button appear.
- **Execute** — Sends `execute.md` as the prompt. Shows current `br list` and the execute prompt preview.
- **System** — Raw JSON of Claude's system init event (model, tools, session info).
- **Command** — The exact `claude` CLI invocation with all flags.
- **State** — Directory listings for `~/.claude/` and `~/.cache/`, plus a `~/.claude.json` summary.
- **Token totals** — Running input/output token counts and cost displayed in the tab bar, accumulated across all runs in the session.
- Press **`s`** to toggle auto-scroll.

## Prompt files

The `prompts/` directory contains three markdown templates served via `GET /api/prompts/{name}`:

- `prompts/plan.md` — Instructions for breaking work into epics in `br`. The user's project description is appended to this template.
- `prompts/refine.md` — Instructions for Claude to read `br` and ask clarifying questions. Sent verbatim (no user text appended).
- `prompts/execute.md` — Instructions for Claude to run `br ready`, implement the next epic, update `br`, and commit. Sent verbatim.

## Building

Requires Go 1.25+. The `web/` directory is embedded into the binary.

```bash
make build
# or directly:
GOOS=darwin GOARCH=arm64 go build .
```

## Running

```bash
# Start the server (requires `claude` CLI on PATH)
./ccwrapper
# Listens on 0.0.0.0:8080

# Replay a saved stream-json file for testing
./ccwrapper --demo example.json
```

## Claude CLI flags

ccwrapper launches Claude with these flags:

- `--print` — non-interactive single-shot mode
- `--output-format stream-json` — structured streaming output
- `--verbose` — include thinking blocks
- `--allow-dangerously-skip-permissions` / `--dangerously-skip-permissions` — no permission prompts
- `--disable-slash-commands` — no slash command processing
- `--no-session-persistence` — don't persist session state
- `--mcp-config ''` / `--strict-mcp-config` — disable MCP servers
- `--disallowed-tools` — disables: `AskUserQuestion`, `CronCreate`, `CronDelete`, `CronList`, `EnterPlanMode`, `ExitPlanMode`, `TodoWrite`, `Skill`, `NotebookEdit`, `EnterWorktree`

## Stack

- **Go** — HTTP server, CLI process management, JSON stream parsing
- **Vanilla JS** — no frameworks, single `app.js`
- **Pico CSS** — minimal dark-theme styling
- **SSE** — real-time event streaming to the browser
