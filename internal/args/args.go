package args

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/markis/gh-copilot/internal/config"
	"github.com/spf13/cobra"
)

// Arguments represents the command-line arguments structure.
type Arguments struct {
	Prompts      []string
	Model        string
	Command      string
	UsePlainText bool
}

// ParseArgs parses command-line arguments and stdin input, returning an Arguments struct.
// It uses Cobra to handle commands and flags, allowing for both predefined commands and direct prompts.
// It reads from stdin if available, and handles errors gracefully.
func ParseArgs(ctx context.Context, cfg config.Config) (Arguments, error) {
	args := Arguments{}

	rootCmd := &cobra.Command{
		Use:   "gh-copilot [command] [flags] [prompt]",
		Short: "A GitHub Copilot CLI tool for AI-assisted development",
		RunE: func(cmd *cobra.Command, cmdArgs []string) error {
			// Handle direct prompts (when no command is specified)
			if len(cmdArgs) > 0 {
				args.Prompts = append(args.Prompts, cmdArgs[0])
			}
			return nil
		},
		SilenceErrors: true, // We'll handle error reporting
		SilenceUsage:  true, // We'll handle usage display
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&args.Model, "model", cfg.Model, "The AI model to use")
	rootCmd.PersistentFlags().BoolVar(&args.UsePlainText, "plain", shouldUsePlainText(cfg), "Disable markdown rendering")

	// Add predefined commands
	for name, prompt := range cfg.Prompts {
		cmdPrompt := prompt // Create a local copy for the closure
		cmd := &cobra.Command{
			Use:   name + " [input]",
			Short: summarizePrompt(cmdPrompt.Prompt),
			RunE: func(cmd *cobra.Command, cmdArgs []string) error {
				args.Command = name
				if len(cmdArgs) > 0 {
					args.Prompts = append(args.Prompts, cmdArgs[0])
				}
				args.Prompts = append(args.Prompts, cmdPrompt.Prompt)
				if cmdPrompt.Model != "" {
					args.Model = cmdPrompt.Model
				}
				return nil
			},
		}
		rootCmd.AddCommand(cmd)
	}

	// Read from stdin if available
	if stat, err := os.Stdin.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
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
		args.Prompts = append(args.Prompts, prompt)
	}

	// Execute the command
	if err := rootCmd.Execute(); err != nil {
		return Arguments{}, err
	}

	// Check if we have any prompts
	if len(args.Prompts) == 0 {
		return Arguments{}, errors.New("no prompt provided")
	}

	return args, nil
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

func summarizePrompt(prompt string) string {
	// Trim and limit the length of the prompt summary
	summary := strings.TrimSpace(prompt)
	if len(summary) > 60 {
		summary = summary[:57] + "..."
	}
	return summary
}
