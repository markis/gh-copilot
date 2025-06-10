package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/markis/gh-copilot/internal/args"
	"github.com/markis/gh-copilot/internal/client"
	"github.com/markis/gh-copilot/internal/config"
)

// main is the entry point of the application. It sets up signal handling for graceful shutdown and runs the main logic.
func main() {
	ctx, shutdown := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer shutdown()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run executes the main logic of the application, loading configuration, parsing arguments, and making API calls.
func run(ctx context.Context) error {
	cfg, err := config.LoadConfig(ctx)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Add timeout to the context from config
	ctx, cancel := context.WithTimeout(ctx, cfg.ContextTimeout)
	defer cancel()

	args, err := args.ParseArgs(ctx, cfg)
	if err != nil {
		return fmt.Errorf("parsing args: %w", err)
	}

	return client.Ask(ctx, cfg, args)
}
