package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type sseClient struct {
	events chan []byte
	done   chan struct{}
}

// claudeArgs are the fixed flags passed to every claude invocation.
var claudeArgs = []string{
	"--output-format", "stream-json",
	"--verbose",
	"--print",
	"--allow-dangerously-skip-permissions",
	"--dangerously-skip-permissions",
	"--disable-slash-commands",
	"--no-session-persistence",
	"--mcp-config", "",
	"--strict-mcp-config",
	"--disallowed-tools", "AskUserQuestion,CronCreate,CronDelete,CronList,EnterPlanMode,ExitPlanMode,TodoWrite,Skill,NotebookEdit,EnterWorktree",
}

type Harness struct {
	mu          sync.Mutex
	running     bool
	cmd         *exec.Cmd
	contextFile  string
	sessionID    string
	demoFile string // if set, replay this file instead of launching claude
	commandDisplay string // pre-computed command string for the Command tab

	clientsMu sync.Mutex
	clients   map[*sseClient]struct{}
}

func NewHarness() *Harness {
	tmpFile, err := os.CreateTemp("", "agentbox-context-*.txt")
	if err != nil {
		log.Fatalf("failed to create context file: %v", err)
	}
	tmpFile.Close()

	return &Harness{
		contextFile:    tmpFile.Name(),
		clients:        make(map[*sseClient]struct{}),
		commandDisplay: formatCommand("claude", claudeArgs),
	}
}

func (h *Harness) Cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cmd != nil && h.cmd.Process != nil {
		h.cmd.Process.Kill()
	}
	os.Remove(h.contextFile)
}

// HandleSSE registers an SSE client and streams events.
func (h *Harness) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	client := &sseClient{
		events: make(chan []byte, 256),
		done:   make(chan struct{}),
	}

	h.clientsMu.Lock()
	h.clients[client] = struct{}{}
	h.clientsMu.Unlock()

	defer func() {
		h.clientsMu.Lock()
		delete(h.clients, client)
		h.clientsMu.Unlock()
		close(client.done)
	}()

	// Send current status and command
	h.mu.Lock()
	running := h.running
	h.mu.Unlock()
	statusJSON, _ := json.Marshal(UIEvent{Type: "status", Running: running})
	fmt.Fprintf(w, "data: %s\n\n", statusJSON)
	cmdJSON, _ := json.Marshal(UIEvent{Type: "command", Content: h.commandDisplay})
	fmt.Fprintf(w, "data: %s\n\n", cmdJSON)
	flusher.Flush()

	for {
		select {
		case data := <-client.events:
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// broadcast sends an event to all connected SSE clients.
func (h *Harness) broadcast(event UIEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()
	for client := range h.clients {
		select {
		case client.events <- data:
		default:
			// Drop if client is slow
		}
	}
}

// HandlePrompt receives a prompt and launches claude.
func (h *Harness) HandlePrompt(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		http.Error(w, "already running", http.StatusConflict)
		return
	}
	h.mu.Unlock()

	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, "invalid prompt", http.StatusBadRequest)
		return
	}

	go h.launch(req.Prompt)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// HandleStop kills the running claude process.
func (h *Harness) HandleStop(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.running || h.cmd == nil || h.cmd.Process == nil {
		http.Error(w, "not running", http.StatusConflict)
		return
	}

	h.cmd.Process.Signal(os.Interrupt)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "stopping"})
}

// HandleClear wipes the context file and notifies clients.
func (h *Harness) HandleClear(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running {
		http.Error(w, "cannot clear while running", http.StatusConflict)
		return
	}

	os.Truncate(h.contextFile, 0)
	h.sessionID = ""

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
}

// HandleState returns directory listings for Claude-related paths.
func (h *Harness) HandleState(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	paths := []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".cache"),
	}
	sections := make([]map[string]string, 0, len(paths))
	for _, p := range paths {
		displayPath := strings.Replace(p, home, "~", 1)
		info, err := os.Stat(p)
		if err != nil {
			sections = append(sections, map[string]string{"path": displayPath, "content": "(not found)"})
			continue
		}
		section := map[string]string{"path": displayPath}
		// Include creation time if available
		if ct := getCreationTime(info); !ct.IsZero() {
			section["created"] = ct.Format(time.RFC3339)
		}
		if !info.IsDir() {
			section["content"] = fmt.Sprintf("%s  %d bytes", info.Name(), info.Size())
			sections = append(sections, section)
			continue
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			section["content"] = "(unreadable: " + err.Error() + ")"
			sections = append(sections, section)
			continue
		}
		var b strings.Builder
		for _, e := range entries {
			ei, _ := e.Info()
			if ei != nil {
				fmt.Fprintf(&b, "%s  %s  %d\n", ei.Mode(), e.Name(), ei.Size())
			} else {
				fmt.Fprintf(&b, "%s\n", e.Name())
			}
		}
		if b.Len() == 0 {
			section["content"] = "(empty)"
		} else {
			section["content"] = b.String()
		}
		sections = append(sections, section)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sections)
}

// HandleClaudeJSON returns the top-level fields of ~/.claude.json.
func (h *Harness) HandleClaudeJSON(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	p := filepath.Join(home, ".claude.json")
	fi, statErr := os.Stat(p)
	var modTime string
	if statErr == nil {
		modTime = fi.ModTime().Format(time.RFC3339)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "invalid JSON: " + err.Error()})
		return
	}
	// Build summary: for each top-level key, show a short representation
	summary := make(map[string]interface{}, len(raw))
	for k, v := range raw {
		var parsed interface{}
		if err := json.Unmarshal(v, &parsed); err == nil {
			switch val := parsed.(type) {
			case map[string]interface{}:
				summary[k] = fmt.Sprintf("{...} (%d keys)", len(val))
			case []interface{}:
				summary[k] = fmt.Sprintf("[...] (%d items)", len(val))
			default:
				summary[k] = val
			}
		} else {
			summary[k] = string(v)
		}
	}
	if modTime != "" {
		summary["_lastModified"] = modTime
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// HandleBr runs 'br list --all' and returns the output.
func (h *Harness) HandleBr(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("br", "list", "--all")
	output, err := cmd.CombinedOutput()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"output": fmt.Sprintf("error: %v\n%s", err, string(output))})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"output": string(output)})
}

// HandleContext returns the current prompt context being sent to claude.
func (h *Harness) HandleContext(w http.ResponseWriter, r *http.Request) {
	data, _ := os.ReadFile(h.contextFile)
	info, _ := os.Stat(h.contextFile)
	var sizeBytes int64
	if info != nil {
		sizeBytes = info.Size()
	}
	result := map[string]interface{}{
		"context":    string(data),
		"file_path":  h.contextFile,
		"size_bytes": sizeBytes,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// launch runs the claude CLI (or replays a demo file) and streams output.
func (h *Harness) launch(prompt string) {
	h.mu.Lock()
	h.running = true
	h.mu.Unlock()

	h.broadcast(UIEvent{Type: "status", Running: true})

	defer func() {
		h.mu.Lock()
		h.running = false
		h.cmd = nil
		h.mu.Unlock()
		h.broadcast(UIEvent{Type: "status", Running: false})
	}()

	// Append user prompt to context file
	f, err := os.OpenFile(h.contextFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		h.broadcast(UIEvent{Type: "error", Content: fmt.Sprintf("context file error: %v", err)})
		return
	}
	fmt.Fprintf(f, "User: %s\n\n", prompt)
	f.Close()

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

		ctxBytes, err := os.ReadFile(h.contextFile)
		if err != nil {
			h.broadcast(UIEvent{Type: "error", Content: fmt.Sprintf("context read error: %v", err)})
			return
		}

		args := make([]string, len(claudeArgs))
		copy(args, claudeArgs)
		args = append(args, "--", string(ctxBytes))
		cmd := exec.Command("claude", args...)
		cmd.Env = os.Environ()

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

	var contextBuf strings.Builder

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

		// Build context entry (lean transcript)
		if entry := BuildContextEntry(ev); entry != "" {
			contextBuf.WriteString(entry)
		}

		// Transform and broadcast to SSE clients
		for _, uiEvent := range TransformEvent(ev) {
			h.broadcast(uiEvent)
		}

		// Small delay in demo mode for visual effect
		h.mu.Lock()
		isDemo := h.demoFile != ""
		h.mu.Unlock()
		if isDemo {
			time.Sleep(150 * time.Millisecond)
		}
	}

	// Append assistant output to context file
	if contextBuf.Len() > 0 {
		f, err := os.OpenFile(h.contextFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			f.WriteString(contextBuf.String())
			f.Close()
		}
	}
}

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

// cleanClaudeJSON strips ~/.claude.json down to only userID and oauthAccount.
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
	for _, key := range []string{"userID", "oauthAccount"} {
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

// formatCommand builds a shell-style command string with one flag per line.
func formatCommand(name string, args []string) string {
	var b strings.Builder
	b.WriteString(name)
	for i := 0; i < len(args); i++ {
		b.WriteString(" \\\n")
		a := args[i]
		if a == "" || strings.ContainsAny(a, " \t\n\"'\\") {
			b.WriteByte('\'')
			b.WriteString(strings.ReplaceAll(a, "'", "'\\''"))
			b.WriteByte('\'')
		} else {
			b.WriteString(a)
		}
		// If next arg is a value (not a flag), keep it on the same line
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			a2 := args[i+1]
			b.WriteByte(' ')
			if a2 == "" || strings.ContainsAny(a2, " \t\n\"'\\") {
				b.WriteByte('\'')
				b.WriteString(strings.ReplaceAll(a2, "'", "'\\''"))
				b.WriteByte('\'')
			} else {
				b.WriteString(a2)
			}
			i++
		}
	}
	return b.String()
}
