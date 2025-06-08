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

func NewParser(ctx context.Context) *Parser {
	return &Parser{
		ctx:    ctx,
		chunks: make(chan Chunk),
	}
}

func (p *Parser) Chunks() <-chan Chunk {
	return p.chunks
}
