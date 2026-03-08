#! /usr/bin/env bash

echo "Deleting most claude state"

# ~/.local has Claude data that seems harmless

rm -rf ~/.claude/todos
rm -rf ~/.claude/tasks
rm -rf ~/.claude/shell-snapshots
rm -rf ~/.claude/session-env
rm -rf ~/.claude/projects
rm -rf ~/.claude/plugins
rm -rf ~/.claude/plans
rm -rf ~/.claude/paste-cache
rm -rf ~/.claude/history.jsonl
rm -rf ~/.claude/file-history
rm -rf ~/.claude/debug
rm -rf ~/.claude/backups
rm -rf ~/.claude/cache
rm -rf ~/.claude/telemetry
rm -rf ~/.claude/mcp-needs-auth-cache.json
rm -rf ~/.claude/settings.json


rm -rf ~/.cache/claude
