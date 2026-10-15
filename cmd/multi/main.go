package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"ggosu/internal/chart"
	_ "ggosu/internal/chart/bms"
	_ "ggosu/internal/chart/ojn"
	_ "ggosu/internal/chart/osu"
	_ "ggosu/internal/chart/sm"
	"ggosu/internal/server/multi"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", "127.0.0.1:8090", "HTTP listen address")
	origins := flag.String("origins", "", "comma-separated exact browser origins (default: same host)")
	clients := flag.Int("clients", 256, "maximum connected clients")
	rooms := flag.Int("rooms", 128, "maximum rooms")
	delay := flag.Duration("start-delay", 3*time.Second, "time between start announcement and game start")
	grace := flag.Duration("finish-grace", 30*time.Second, "result reporting grace after the chart ends")
	songs := flag.String("songs", "", "canonical chart folder for result verification (empty: provisional results)")
	flag.Parse()
	config := multi.Config{MaxClients: *clients, MaxRooms: *rooms, StartDelay: *delay, FinishGrace: *grace}
	if *songs != "" {
		catalog, err := chart.Scan(*songs, nil)
		if err != nil {
			return fmt.Errorf("scan server charts: %w", err)
		}
		if len(catalog.Errors) > 0 || len(catalog.Entries) == 0 {
			return fmt.Errorf("server chart catalogue is empty or invalid: %v", catalog.Errors)
		}
		files := map[string][]*chart.Chart{}
		for _, e := range catalog.Entries {
			cs, ok := files[e.Path]
			if !ok {
				cs, err = chart.Load(e.Path)
				if err != nil {
					return err
				}
				files[e.Path] = cs
			}
			config.Charts = append(config.Charts, cs[e.Index])
		}
	}
	if *origins != "" {
		for _, origin := range strings.Split(*origins, ",") {
			config.Origins = append(config.Origins, strings.TrimSpace(origin))
		}
	}
	h, err := multi.NewServer(config)
	if err != nil {
		return err
	}
	defer h.Close()
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("listen for multiplayer: %w", err)
	}
	server := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	fmt.Println("Gosu multiplayer: http://" + listener.Addr().String())
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve multiplayer: %w", err)
	case <-ctx.Done():
		// Upgraded sockets belong to multi.Server, not http.Server.Shutdown.
		if err := h.Close(); err != nil {
			return err
		}
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}
