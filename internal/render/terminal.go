package render

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/cli/go-gh/v2/pkg/markdown"
	"github.com/markis/gh-copilot/internal/args"
	"github.com/markis/gh-copilot/internal/config"
	"github.com/markis/gh-copilot/internal/stream"
)

// TerminalRenderer is responsible for rendering markdown content to the terminal.
type TerminalRenderer struct {
	ctx       context.Context
	markdown  *glamour.TermRenderer
	plainText bool
	buffer    strings.Builder
	inBlock   bool // Track if we are currently in a block element (e.g., code block, table, etc.)
}

// NewTerminalRenderer creates a new TerminalRenderer instance.
func NewTerminalRenderer(ctx context.Context, cfg config.Config, args args.Arguments) (*TerminalRenderer, error) {
	var md *glamour.TermRenderer
	var err error

	// use plain text rendering if specified in arguments
	if !args.UsePlainText {
		options := make([]glamour.TermRendererOption, 0, 2)
		if cfg.Render.WrapLines && cfg.Render.WrapWidth >= 0 {
			options = append(options, markdown.WithWrap(cfg.Render.WrapWidth))
		}
		if cfg.Render.Theme != "" {
			options = append(options, glamour.WithStandardStyle(cfg.Render.Theme))
		}

		md, err = glamour.NewTermRenderer(options...)
		if err != nil {
			return nil, fmt.Errorf("creating markdown renderer: %w", err)
		}
	}

	return &TerminalRenderer{
		ctx:       ctx,
		markdown:  md,
		plainText: args.UsePlainText,
	}, nil
}

// Render processes the stream of chunks and renders them to the terminal.
func (t *TerminalRenderer) Render(chunks <-chan stream.Chunk) error {
	done := t.ctx.Done()
	for {
		select {
		case <-done:
			return t.ctx.Err()

		case chunk, ok := <-chunks:
			if !ok {
				// Channel closed, render remaining content
				return t.renderRemaining()
			}

			if chunk.Error != nil {
				return fmt.Errorf("stream error: %w", chunk.Error)
			}

			if err := t.processChunk(chunk.Content); err != nil {
				return fmt.Errorf("failed to process chunk: %w", err)
			}
		}
	}
}

// processChunk processes the incoming content chunk, checking for markdown break points
func (t *TerminalRenderer) processChunk(content string) error {
	t.buffer.WriteString(content)
	bufContent := t.buffer.String()

	if idx := t.findMarkdownBreakPoint(bufContent); idx > -1 {
		if err := t.renderContent(bufContent[:idx]); err != nil {
			return err
		}
		// Reset buffer with remaining content
		remaining := bufContent[idx:]
		t.buffer.Reset()
		t.buffer.WriteString(remaining)
	}
	return nil
}

// renderRemaining checks if there's any content left in the buffer and renders it.
func (t *TerminalRenderer) renderRemaining() error {
	if remaining := t.buffer.String(); remaining != "" {
		if err := t.renderContent(remaining); err != nil {
			return fmt.Errorf("failed to render remaining content: %w", err)
		}
	}
	fmt.Println()
	return nil
}

// renderContent processes and prints the content, handling both plain text and markdown rendering.
func (t *TerminalRenderer) renderContent(content string) error {
	if t.plainText {
		fmt.Print(content)
		return nil
	}

	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "#") {
		fmt.Println()
	}

	mdContent, err := t.markdown.Render(content)
	if err != nil {
		return fmt.Errorf("failed to render markdown: %w", err)
	}

	fmt.Println(strings.TrimSpace(mdContent))
	return nil
}

// findMarkdownBreakPoint finds the last occurrence of a markdown break point in the content,
// ignoring any breakpoints that occur within block elements.
func (t *TerminalRenderer) findMarkdownBreakPoint(content string) int {
	lines := strings.Split(content, "\n")

	inCodeBlock := false
	inList := false
	inTable := false
	inBlockquote := false
	lastBreakPosition := -1

	position := 0
	for i, line := range lines {
		lineLength := len(line) + 1 // +1 for newline
		trimmed := strings.TrimSpace(line)

		// Check for code block delimiters
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
		}

		// Skip processing if in code block
		if !inCodeBlock {
			// Check for table rows
			if strings.HasPrefix(trimmed, "|") && strings.Contains(trimmed, "|") {
				inTable = true
			} else if inTable && trimmed == "" {
				inTable = false
				// Add a break after table ends
				lastBreakPosition = position
			}

			// Check for blockquotes
			if strings.HasPrefix(trimmed, ">") {
				inBlockquote = true
			} else if inBlockquote && trimmed == "" {
				inBlockquote = false
				// Add a break after blockquote ends
				lastBreakPosition = position
			}

			// Check for list items
			listPrefix := strings.HasPrefix(trimmed, "- ") ||
				strings.HasPrefix(trimmed, "* ") ||
				strings.HasPrefix(trimmed, "+ ") ||
				(len(trimmed) > 0 && strings.Contains("0123456789", string(trimmed[0])) &&
					strings.Contains(trimmed, ". "))

			if listPrefix {
				inList = true
			} else if inList && trimmed == "" {
				inList = false
				// Add a break after list ends
				lastBreakPosition = position
			}

			// Find break points (paragraphs, headers, etc.)
			currentInBlock := inCodeBlock || inTable || inBlockquote || inList

			// Consider a break point when we're not in a block and find an empty line
			// followed by a non-empty line or the end of content
			if !currentInBlock && trimmed == "" && i > 0 {
				// If next line exists and is not empty, or this is the last line
				if (i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "") || i == len(lines)-1 {
					lastBreakPosition = position
				}
			}

			// Also break at headers for better rendering
			if !currentInBlock && strings.HasPrefix(trimmed, "#") {
				if position > 0 { // Don't break at the very beginning
					lastBreakPosition = position - lineLength
				}
			}
		}

		position += lineLength
	}

	return lastBreakPosition
}
