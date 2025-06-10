package args

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/markis/gh-copilot/internal/config"
)

// Arguments represents the command-line arguments structure.
type Arguments struct {
	Prompts      []string
	Model        string
	Command      string
	UsePlainText bool
}

// parseArgs parses command-line arguments and stdin input.
func ParseArgs(ctx context.Context, cfg config.Config) (Arguments, error) {
	var model string
	var command string
	var plainText bool
	flag.StringVar(&model, "model", cfg.Model, "The AI model to use")
	flag.StringVar(&command, "c", "", "Use a predefined command from config")
	flag.BoolVar(&plainText, "plain", shouldUsePlainText(cfg), "Disable markdown rendering")
	flag.Parse()

	prompts := make([]string, 0, 2)
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
		prompt := strings.TrimSpace(buf.String())
		prompts = append(prompts, prompt)
	}
	if flag.NArg() > 0 {
		prompt := flag.Arg(0)
		prompts = append(prompts, prompt)
	}

	if command == "" && len(prompts) == 0 {
		return Arguments{}, errors.New("no prompt or command provided")
	}

	if command != "" && len(prompts) > 0 {
		cmdPrompt, ok := cfg.Prompts[command]
		if !ok {
			return Arguments{}, fmt.Errorf("command '%s' not found in config", command)
		}
		prompts = append(prompts, cmdPrompt.Prompt)

		if cmdPrompt.Model != "" {
			model = cmdPrompt.Model
		}
	}

	return Arguments{Prompts: prompts, Model: model, Command: command, UsePlainText: plainText}, nil
}

// shouldUsePlainText determines if plain text output should be used based on environment and terminal settings.
func shouldUsePlainText(cfg config.Config) bool {
	// Check if the rendering format is set to plain
	if cfg.Render.Format == "plain" {
		return true
	}

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
