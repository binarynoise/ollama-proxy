package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ollama-proxy/internal/proxy"
	"ollama-proxy/internal/tracker"
	"ollama-proxy/internal/tui"
)

func main() {
	// Parse command line flags
	listenAddr := flag.String("listen", ":11444", "Address to listen on")
	targetURL := flag.String("target", "http://localhost:11434", "Ollama API URL")
	maxCalls := flag.Int("max-calls", 50, "Maximum number of calls to keep in history")
	flag.Parse()

	// Create a context that will be canceled on interrupt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	// Initialize components
	tracker := tracker.NewCallTracker(*maxCalls)
	defer tracker.Close()

	// Create and start the proxy
	proxy, err := proxy.NewProxy(*targetURL, tracker)
	if err != nil {
		log.Fatalf("Failed to create proxy: %v", err)
	}

	server := &http.Server{
		Addr:    *listenAddr,
		Handler: proxy,
	}

	// Start the HTTP server in a goroutine
	go func() {
		log.Printf("Starting proxy server on %s, forwarding to %s\n", *listenAddr, *targetURL)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Create and start the TUI in a goroutine
	tuiApp := tui.NewTUI(tracker)
	tuiDone := make(chan struct{})
	go func() {
		defer close(tuiDone)
		if err := tuiApp.Run(); err != nil {
			log.Printf("TUI error: %v", err)
		}
		// When TUI exits, cancel the context to trigger server shutdown
		cancel()
	}()

	// Wait for either context cancellation or TUI exit
	select {
	case <-ctx.Done():
		// Context was cancelled (e.g., by signal)
	case <-tuiDone:
		// TUI was closed by user
	}

	// Shutdown the server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}
}
