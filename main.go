package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

//go:embed web/*
var webFS embed.FS

func main() {
	demoFile := flag.String("demo", "", "replay a stream-json file instead of launching claude")
	flag.Parse()

	h := NewHarness()
	if *demoFile != "" {
		h.demoFile = *demoFile
		fmt.Printf("demo mode: replaying %s\n", *demoFile)
	}
	defer h.Cleanup()

	// Cleanup on signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		h.Cleanup()
		os.Exit(0)
	}()

	// Static files
	webSub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServer(http.FS(webSub)))
	mux.HandleFunc("GET /events", h.HandleSSE)
	mux.HandleFunc("POST /api/prompt", h.HandlePrompt)
	mux.HandleFunc("POST /api/stop", h.HandleStop)
	mux.HandleFunc("POST /api/clear", h.HandleClear)
	mux.HandleFunc("GET /api/context", h.HandleContext)

	addr := "0.0.0.0:8080"
	fmt.Printf("agentbox listening on http://%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
