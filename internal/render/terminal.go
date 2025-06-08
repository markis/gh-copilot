package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/cli/go-gh/v2/pkg/markdown"
	"github.com/markis/gh-copilot/internal/stream"
)

type TerminalRenderer struct {
	markdown  *glamour.TermRenderer
	plainText bool
	buffer    strings.Builder
}

func NewTerminalRenderer(usePlainText bool) *TerminalRenderer {
	var md *glamour.TermRenderer
	if !usePlainText {
		md, _ = glamour.NewTermRenderer(
			markdown.WithWrap(120),
			glamour.WithAutoStyle(),
		)
	}

	return &TerminalRenderer{
		markdown:  md,
		plainText: usePlainText,
	}
}

func (t *TerminalRenderer) Render(chunks <-chan stream.Chunk) error {
	for chunk := range chunks {
		if chunk.Error != nil {
			return fmt.Errorf("stream error: %w", chunk.Error)
		}

		t.buffer.WriteString(chunk.Content)
		content := t.buffer.String()

		if idx := findMarkdownBreakPoint(content); idx > 0 {
			if err := t.renderContent(content[:idx]); err != nil {
				return err
			}
			// Reset buffer with remaining content
			remaining := content[idx:]
			t.buffer.Reset()
			t.buffer.WriteString(remaining)
		}
	}

	// Render any remaining content
	if remaining := t.buffer.String(); remaining != "" {
		if err := t.renderContent(remaining); err != nil {
			return err
		}
	}

	fmt.Println()
	return nil
}

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

func findMarkdownBreakPoint(content string) int {
	const marker string = "\n\n"
	lastBreak := -1
	idx := strings.LastIndex(content, marker)
	if idx > lastBreak {
		lastBreak = idx + len(marker)
	}
	return lastBreak
}
