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
	ctx := context.Background()

	// Set up signal handling
	ctx, cancel := context.WithCancel(ctx)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nReceived termination signal. Shutting down...")
		cancel()
	}()

	// set up a timeout context
	ctx, cancelTimeout := context.WithTimeout(ctx, timeoutDuration)
	defer cancelTimeout()

	args, err := args.ParseArgs(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var prompt string
	if args.Command != "" {
		config, err := config.LoadConfig(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}

		cmdPrompt, ok := config.Prompts[args.Command]
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: command '%s' not found in config\n", args.Command)
			os.Exit(1)
		}
		prompt = cmdPrompt

		if args.Prompt != "" {
			prompt += "\n" + args.Prompt
		}
	} else {
		prompt = args.Prompt
	}

	if err := client.Ask(ctx, prompt, args.Model, args.PlainText); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
