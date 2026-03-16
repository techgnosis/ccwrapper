package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cleanClaudeState removes ~/.claude (preserving credentials.json) and ~/.cache/claude.
func cleanClaudeState() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	claudeDir := filepath.Join(home, ".claude")
	credsPath := filepath.Join(claudeDir, "credentials.json")

	// Back up credentials.json if it exists
	var creds []byte
	creds, err = os.ReadFile(credsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read credentials: %w", err)
	}

	if err := os.RemoveAll(claudeDir); err != nil {
		return fmt.Errorf("remove %s: %w", claudeDir, err)
	}

	// Restore credentials.json if we backed it up
	if creds != nil {
		if err := os.MkdirAll(claudeDir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", claudeDir, err)
		}
		if err := os.WriteFile(credsPath, creds, 0644); err != nil {
			return fmt.Errorf("restore credentials: %w", err)
		}
	}

	// Remove cache
	cacheDir := filepath.Join(home, ".cache", "claude")
	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("remove %s: %w", cacheDir, err)
	}

	return nil
}

// cleanClaudeJSON strips ~/.claude.json down to only oauthAccount.
func cleanClaudeJSON() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".claude.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var full map[string]json.RawMessage
	if err := json.Unmarshal(data, &full); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	keep := make(map[string]json.RawMessage)
	for _, key := range []string{"oauthAccount"} {
		if v, ok := full[key]; ok {
			keep[key] = v
		}
	}

	out, err := json.MarshalIndent(keep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// shellQuote returns s single-quoted if it contains special characters.
func shellQuote(s string) string {
	if s == "" || strings.ContainsAny(s, " \t\n\"'\\") {
		return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
	}
	return s
}

// formatCommand builds a shell-style command string with one flag per line.
func formatCommand(name string, args []string) string {
	var b strings.Builder
	b.WriteString(name)
	for i := 0; i < len(args); i++ {
		b.WriteString(" \\\n")
		b.WriteString(shellQuote(args[i]))
		// If next arg is a value (not a flag), keep it on the same line
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			b.WriteByte(' ')
			b.WriteString(shellQuote(args[i+1]))
			i++
		}
	}
	return b.String()
}
