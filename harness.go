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

type Harness struct {
	mu          sync.Mutex
	running     bool
	cmd         *exec.Cmd
	contextFile  string
	sessionID    string
	demoFile string // if set, replay this file instead of launching claude

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
		contextFile: tmpFile.Name(),
		clients:     make(map[*sseClient]struct{}),
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

	// Send current status
	h.mu.Lock()
	running := h.running
	h.mu.Unlock()
	statusJSON, _ := json.Marshal(UIEvent{Type: "status", Running: running})
	fmt.Fprintf(w, "data: %s\n\n", statusJSON)
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
		filepath.Join(home, ".claude.json"),
		filepath.Join(home, ".cache"),
	}
	sections := make([]map[string]string, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			sections = append(sections, map[string]string{"path": p, "content": "(not found)"})
			continue
		}
		if !info.IsDir() {
			sections = append(sections, map[string]string{"path": p, "content": fmt.Sprintf("%s  %d bytes", info.Name(), info.Size())})
			continue
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			sections = append(sections, map[string]string{"path": p, "content": "(unreadable: " + err.Error() + ")"})
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
			sections = append(sections, map[string]string{"path": p, "content": "(empty)"})
		} else {
			sections = append(sections, map[string]string{"path": p, "content": b.String()})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sections)
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
		cleanCmd := exec.Command("bash", "-c", string(cleanScript))
		if out, err := cleanCmd.CombinedOutput(); err != nil {
			log.Printf("clean-claude.sh: %v: %s", err, out)
		}

		ctxBytes, err := os.ReadFile(h.contextFile)
		if err != nil {
			h.broadcast(UIEvent{Type: "error", Content: fmt.Sprintf("context read error: %v", err)})
			return
		}

		args := []string{
			"--output-format", "stream-json",
			"--verbose",
			"--print",
			"--allow-dangerously-skip-permissions",
			"--dangerously-skip-permissions",
			"--disable-slash-commands",
			"--no-session-persistence",
			"--mcp-config", "",
			"--strict-mcp-config",
			"--disallowed-tools", "AskUserQuestion,CronCreate,CronDelete,CronList,EnterPlanMode,ExitPlanMode,TodoWrite,Skill,NotebookEdit,Glob,Grep,EnterWorktree",
		}
		// Broadcast the flags for the Command tab (before appending prompt)
		cmdDisplay := string(cleanScript) + "\n"
		cmdDisplay += formatCommand("claude", args)
		h.broadcast(UIEvent{Type: "command", Content: cmdDisplay})

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

// formatCommand builds a shell-style command string with one flag per line.
func formatCommand(name string, args []string) string {
	var b strings.Builder
	b.WriteString(name)
	for i := 0; i < len(args); i++ {
		b.WriteString(" \\\n  ")
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
