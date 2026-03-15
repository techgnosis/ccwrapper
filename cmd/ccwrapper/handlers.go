package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

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
	statusJSON, err := json.Marshal(UIEvent{Type: UITypeStatus, Running: running})
	if err != nil {
		log.Printf("marshal status event: %v", err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", statusJSON)
	cmdJSON, err := json.Marshal(UIEvent{Type: UITypeCommand, Content: h.commandDisplay})
	if err != nil {
		log.Printf("marshal command event: %v", err)
		return
	}
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

// HandlePrompt receives a prompt and launches claude.
func (h *Harness) HandlePrompt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Prompt) == "" {
		jsonError(w, "invalid prompt", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		jsonError(w, "already running", http.StatusConflict)
		return
	}
	h.running = true
	h.mu.Unlock()

	go h.launch(req.Prompt)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// HandleStop kills the running claude process.
func (h *Harness) HandleStop(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.running || h.cmd == nil || h.cmd.Process == nil {
		jsonError(w, "not running", http.StatusConflict)
		return
	}

	h.cmd.Process.Signal(os.Interrupt)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "stopping"})
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

// HandlePromptFile returns the contents of a prompt file (e.g. prompts/plan.md).
func (h *Harness) HandlePromptFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		jsonError(w, "missing name", http.StatusBadRequest)
		return
	}
	path := filepath.Join("prompts", name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"content": "", "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"content": string(data)})
}

// HandleBrList runs "br list" and returns its output plus a count of open issues.
func (h *Harness) HandleBrList(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("br", "list")
	out, err := cmd.CombinedOutput()
	if err != nil {
		jsonError(w, fmt.Sprintf("br list failed: %v: %s", err, string(out)), http.StatusInternalServerError)
		return
	}

	// Count open issues
	openCount := 0
	openCmd := exec.Command("br", "list", "--json", "--status=open")
	openOut, err := openCmd.CombinedOutput()
	if err != nil {
		log.Printf("br list --json --status=open: %v", err)
	} else {
		var issues []interface{}
		if err := json.Unmarshal(openOut, &issues); err != nil {
			log.Printf("unmarshal br list output: %v", err)
		} else {
			openCount = len(issues)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"output":     string(out),
		"open_count": openCount,
	})
}

// HandleBrScrap hard-deletes all open work items from br.
func (h *Harness) HandleBrScrap(w http.ResponseWriter, r *http.Request) {
	// Get all open issue IDs
	listCmd := exec.Command("br", "list", "--json", "--status=open")
	listOut, err := listCmd.CombinedOutput()
	if err != nil {
		jsonError(w, fmt.Sprintf("br list failed: %v: %s", err, string(listOut)), http.StatusInternalServerError)
		return
	}

	var issues []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(listOut, &issues); err != nil {
		jsonError(w, fmt.Sprintf("parse br list: %v", err), http.StatusInternalServerError)
		return
	}

	if len(issues) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"deleted": 0,
		})
		return
	}

	// Collect IDs and hard-delete them
	ids := make([]string, len(issues))
	for i, issue := range issues {
		ids[i] = issue.ID
	}

	delArgs := append([]string{"delete", "--hard", "--force"}, ids...)
	delCmd := exec.Command("br", delArgs...)
	delOut, err := delCmd.CombinedOutput()
	if err != nil {
		jsonError(w, fmt.Sprintf("br delete failed: %v: %s", err, string(delOut)), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deleted": len(ids),
	})
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
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), http.StatusInternalServerError)
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
