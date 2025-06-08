package render

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/cli/go-gh/v2/pkg/markdown"
	"github.com/markis/gh-copilot/internal/stream"
)

// TerminalRenderer is responsible for rendering markdown content to the terminal.
type TerminalRenderer struct {
	ctx       context.Context
	markdown  *glamour.TermRenderer
	plainText bool
	buffer    strings.Builder
}

// NewTerminalRenderer creates a new TerminalRenderer instance.
func NewTerminalRenderer(ctx context.Context, usePlainText bool) *TerminalRenderer {
	var md *glamour.TermRenderer
	if !usePlainText {
		md, _ = glamour.NewTermRenderer(
			markdown.WithWrap(120),
			glamour.WithAutoStyle(),
		)
	}

	return &TerminalRenderer{
		ctx:       ctx,
		markdown:  md,
		plainText: usePlainText,
	}
}

// Render processes the stream of chunks and renders them to the terminal.
func (t *TerminalRenderer) Render(chunks <-chan stream.Chunk) error {
	for {
		select {
		case <-t.ctx.Done():
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

	if idx := findMarkdownBreakPoint(bufContent); idx > 0 {
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

// findMarkdownBreakPoint finds the last occurrence of a markdown break point in the content.
func findMarkdownBreakPoint(content string) int {
	const marker string = "\n\n"
	lastBreak := -1
	idx := strings.LastIndex(content, marker)
	if idx > lastBreak {
		lastBreak = idx + len(marker)
	}
	return lastBreak
}
