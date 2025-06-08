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
	done := p.ctx.Done()

	reader := bufio.NewReaderSize(body, 4096)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	scanner.Split(bufio.ScanLines)

	var chunk ChatResponse

	for {
		select {
		case <-done:
			p.chunks <- Chunk{Error: p.ctx.Err()}
			return
		default:
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					p.chunks <- Chunk{Error: err}
				}
				return
			}

			line := scanner.Text()
			if line == "" || line == "data: [DONE]" {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				p.chunks <- Chunk{Error: err}
				continue
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
		}
	}
}
