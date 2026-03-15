package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"sync"
)

// jsonError writes a JSON error response with the given status code.
func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

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
	mu             sync.Mutex
	running        bool
	cmd            *exec.Cmd
	sessionID      string
	demoFile       string // if set, replay this file instead of launching claude
	commandDisplay string // pre-computed command string for the Command tab

	clientsMu sync.Mutex
	clients   map[*sseClient]struct{}
}

func NewHarness() *Harness {
	return &Harness{
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
}

// broadcast sends an event to all connected SSE clients.
func (h *Harness) broadcast(event UIEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("marshal broadcast event: %v", err)
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
