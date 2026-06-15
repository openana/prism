package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openana/prism/pkg/config"
	"github.com/openana/prism/pkg/server"
)

func main() {
	if !Main() {
		os.Exit(1)
	}
}

func Main() bool {
	defer initProfiles()()

	configPath := flag.String("config", "config.yaml", "path to YAML config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		return false
	}

	srv, cleanup, err := server.InitializeServer(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize server: %v\n", err)
		return false
	}
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server run error: %v\n", err)
		return false
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Stop(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "server stop error: %v\n", err)
	}

	return true
}
