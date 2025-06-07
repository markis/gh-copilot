package main

import (
	"fmt"
	"os"
)

func main() {
	args, err := parseArgs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var prompt string
	if args.Command != "" {
		config, err := loadConfig()
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

	if err := ask(prompt, args.Model, args.PlainText); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
