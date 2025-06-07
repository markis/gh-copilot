package args

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
	Prompt    string
	Model     string
	Command   string
	PlainText bool
}

// parseArgs parses command-line arguments and stdin input.
func ParseArgs() (Arguments, error) {
	var model string
	var command string
	var plainText bool
	flag.StringVar(&model, "model", "claude-3.7-sonnet", "The AI model to use")
	flag.StringVar(&command, "c", "", "Use a predefined command from config")
	flag.BoolVar(&plainText, "plain", shouldUsePlainText(), "Disable markdown rendering")
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

	if command == "" && prompt == "" {
		return Arguments{}, errors.New("no prompt or command provided")
	}

	return Arguments{Prompt: prompt, Model: model, Command: command, PlainText: plainText}, nil
}

// shouldUsePlainText determines if plain text output should be used based on environment and terminal settings.
func shouldUsePlainText() bool {
	// Check if output is being redirected
	if fileInfo, _ := os.Stdout.Stat(); fileInfo != nil {
		if (fileInfo.Mode() & os.ModeCharDevice) == 0 {
			return true
		}
	}

	// Check for NO_COLOR environment variable
	if _, exists := os.LookupEnv("NO_COLOR"); exists {
		return true
	}

	// Check for TERM=dumb
	if term := os.Getenv("TERM"); term == "dumb" {
		return true
	}

	return false
}
