package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/markis/gh-copilot/internal/args"
	"github.com/markis/gh-copilot/internal/client"
	"github.com/markis/gh-copilot/internal/config"
)

// timeoutDuration defines how long the program will wait for a response before timing out.
const timeoutDuration = 5 * time.Minute

func main() {
	ctx, shutdown := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer shutdown()

	// Add timeout to the context
	ctx, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	args, err := args.ParseArgs(ctx)
	if err != nil {
		return fmt.Errorf("parsing args: %w", err)
	}

	prompt, err := buildPrompt(ctx, args)
	if err != nil {
		return fmt.Errorf("building prompt: %w", err)
	}

	return client.Ask(ctx, prompt, args.Model, args.PlainText)
}

func buildPrompt(ctx context.Context, args args.Arguments) (string, error) {
	if args.Command == "" {
		return args.Prompt, nil
	}

	config, err := config.LoadConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("loading config: %w", err)
	}

	cmdPrompt, ok := config.Prompts[args.Command]
	if !ok {
		return "", fmt.Errorf("command '%s' not found in config", args.Command)
	}

	if args.Prompt == "" {
		return cmdPrompt, nil
	}

	return cmdPrompt + "\n" + args.Prompt, nil
}
