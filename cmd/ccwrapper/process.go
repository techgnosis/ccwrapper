package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// launch runs the claude CLI (or replays a demo file) and streams output.
func (h *Harness) launch(prompt string) {
	h.broadcast(UIEvent{Type: "status", Running: true})

	defer func() {
		h.mu.Lock()
		h.running = false
		h.cmd = nil
		h.mu.Unlock()
		h.broadcast(UIEvent{Type: "status", Running: false})
	}()

	var reader io.Reader

	if h.demoFile != "" {
		// Demo mode: replay file with a small delay per line
		demoF, err := os.Open(h.demoFile)
		if err != nil {
			h.broadcast(UIEvent{Type: "error", Content: fmt.Sprintf("demo file error: %v", err)})
			return
		}
		defer demoF.Close()
		reader = demoF
	} else {
		// Real mode: clean state then launch claude CLI
		if err := cleanClaudeState(); err != nil {
			log.Printf("clean claude state: %v", err)
		}

		// Strip ~/.claude.json to only userID and oauthAccount
		if err := cleanClaudeJSON(); err != nil {
			log.Printf("clean claude.json: %v", err)
		}

		args := make([]string, len(claudeArgs))
		copy(args, claudeArgs)
		args = append(args, "--", prompt)
		cmd := exec.Command("claude", args...)

		var stderrBuf strings.Builder
		cmd.Stderr = &stderrBuf

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			h.broadcast(UIEvent{Type: "error", Content: fmt.Sprintf("pipe error: %v", err)})
			return
		}

		h.mu.Lock()
		h.cmd = cmd
		h.mu.Unlock()

		if err := cmd.Start(); err != nil {
			h.broadcast(UIEvent{Type: "error", Content: fmt.Sprintf("start error: %v", err)})
			return
		}
		defer func() {
			cmd.Wait()
			if s := strings.TrimSpace(stderrBuf.String()); s != "" {
				log.Printf("claude stderr: %s", s)
				h.broadcast(UIEvent{Type: "error", Content: "claude stderr: " + s})
			}
		}()
		reader = stdout
	}

	h.processStream(reader)
}

// processStream reads stream-json lines from a reader and broadcasts events.
func (h *Harness) processStream(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		ev, err := ParseEvent(line)
		if err != nil {
			log.Printf("parse error: %v (line: %s)", err, truncate(string(line), 100))
			continue
		}

		// Capture session ID
		if ev.Type == "system" && ev.SessionID != "" {
			h.mu.Lock()
			h.sessionID = ev.SessionID
			h.mu.Unlock()
		}

		// Transform and broadcast to SSE clients
		for _, uiEvent := range TransformEvent(ev) {
			h.broadcast(uiEvent)
		}

		// Small delay in demo mode for visual effect
		if h.demoFile != "" {
			time.Sleep(150 * time.Millisecond)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("stream scanner error: %v", err)
	}
}
