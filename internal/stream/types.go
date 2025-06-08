package stream

import "context"

// Chunk represents a processed piece of content from the stream
type Chunk struct {
	Content string
	Done    bool
	Error   error
}

// Parser handles the processing of raw stream data into chunks
type Parser struct {
	ctx    context.Context
	chunks chan Chunk
}

// NewParser creates a new Parser instance with a context and a channel for chunks
func NewParser(ctx context.Context) *Parser {
	return &Parser{
		ctx:    ctx,
		chunks: make(chan Chunk),
	}
}

// Chunks returns a read-only channel that emits processed chunks
func (p *Parser) Chunks() <-chan Chunk {
	return p.chunks
}
