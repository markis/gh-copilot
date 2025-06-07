package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// Arguments represents the command-line arguments structure.
type Arguments struct {
	Prompt string
	Model  string
}

// parseArgs parses command-line arguments and stdin input.
func parseArgs() (Arguments, error) {
	var model string
	flag.StringVar(&model, "model", "claude-3.7-sonnet", "The AI model to use")
	flag.Parse()

	var prompt string
	if stat, err := os.Stdin.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		// Piped input
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max buffer
		var buf strings.Builder
		for scanner.Scan() {
			buf.WriteString(scanner.Text())
			buf.WriteByte('\n')
		}
		if err := scanner.Err(); err != nil {
			return Arguments{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		prompt = strings.TrimSpace(buf.String())
	} else if flag.NArg() > 0 {
		prompt = flag.Arg(0)
	}

	if prompt == "" {
		return Arguments{}, errors.New("no prompt provided")
	}

	return Arguments{Prompt: prompt, Model: model}, nil
}
