package stream

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// ChatResponse represents the structure of the response from the chat API.
type ChatResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (p *Parser) Process(body io.ReadCloser) {
	defer close(p.chunks)

	reader := bufio.NewReaderSize(body, 4096)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	scanner.Split(bufio.ScanLines)

	var (
		chunk ChatResponse
		done  = p.ctx.Done()
	)

	for {
		select {
		case <-done:
			p.chunks <- Chunk{Error: p.ctx.Err()}
			return
		default:
			if ok := p.processChunk(scanner, chunk); !ok {
				return
			}
		}
	}
}

func (p *Parser) processChunk(scanner *bufio.Scanner, chunk ChatResponse) bool {
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			p.chunks <- Chunk{Error: err}
		}
		return false
	}

	line := scanner.Text()

	// Fast path for empty lines and done marker
	switch line {
	case "", "data: [DONE]":
		return true
	}

	data := strings.TrimPrefix(line, "data: ")
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		p.chunks <- Chunk{Error: err}
		return true
	}

	if len(chunk.Choices) > 0 {
		content := chunk.Choices[0].Delta.Content
		if content == "" {
			content = chunk.Choices[0].Message.Content
		}
		if content != "" {
			p.chunks <- Chunk{Content: content}
		}
	}
	return true
}
