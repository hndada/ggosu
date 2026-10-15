package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"ggosu/internal/server/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	songs := flag.String("songs", "songs", "chart directory")
	dist := flag.String("wasm", "build/web", "compiled web files")
	scores := flag.String("scores", "data/scores", "verified submitted score directory")
	archive := flag.String("archive-server", "http://127.0.0.1:8091", "song archive catalogue server")
	flag.Parse()
	for _, name := range []string{"game.wasm", "wasm_exec.js"} {
		if _, err := os.Stat(filepath.Join(*dist, name)); err != nil {
			return fmt.Errorf("web build missing; run pwsh scripts/buildweb.ps1: %w", err)
		}
	}
	h, err := web.NewHandler(web.Config{Songs: *songs, WASM: *dist, Scores: *scores, Archive: *archive})
	if err != nil {
		return err
	}
	defer h.Close()
	server := &http.Server{Addr: *addr, Handler: h, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	fmt.Println("Gosu web: http://" + *addr)
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}
