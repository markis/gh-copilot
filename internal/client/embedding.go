package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"

	"github.com/markis/gh-copilot/internal/config"
)

// EmbeddingInput represents an input for embedding generation
type EmbeddingInput struct {
	Filename  string
	Content   string
	Outline   string
	Filetype  string
	StartLine int
}

// EmbeddingOutput represents the result from embedding generation
type EmbeddingOutput struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

// prepareEmbeddingRequest prepares the request payload for embeddings
func prepareEmbeddingRequest(inputs []EmbeddingInput, threshold int) []string {
	results := make([]string, 0, len(inputs))

	for _, input := range inputs {
		content := input.Content
		if input.Outline != "" && len(content) > threshold {
			content = input.Outline
		}
		if len(content) > threshold {
			content = content[:threshold] + "\n... (truncated)"
		}

		if input.Filetype == "raw" {
			results = append(results, content)
		} else {
			formatted := fmt.Sprintf("File: `%s`\n```%s\n%s\n```",
				input.Filename,
				input.Filetype,
				content)
			results = append(results, formatted)
		}
	}

	return results
}

// GenerateEmbeddings generates embeddings for the provided inputs
//
// Here's how you would use it in practice:
//
// ```go
// // First, generate embeddings for your document collection
//
//	documents := []EmbeddingInput{
//	    {
//	        Filename: "file1.go",
//	        Content: "content1",
//	        Filetype: "go",
//	    },
//	    {
//	        Filename: "file2.go",
//	        Content: "content2",
//	        Filetype: "go",
//	    },
//	}
//
// documentEmbeddings, err := GenerateEmbeddings(ctx, documents, "copilot-codex")
//
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// // When you have a new query, generate its embedding
//
//	queryDoc := []EmbeddingInput{{
//	    Content: "your query here",
//	    Filetype: "raw",
//	}}
//
// queryEmbedding, err := GenerateEmbeddings(ctx, queryDoc, "copilot-codex")
//
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// // Find similar documents
// matches := FindSimilarDocuments(queryEmbedding[0], documents, documentEmbeddings, 0.8)
//
// // Use the most relevant matches in your chat context
// relevantDocs := make([]EmbeddingInput, 0)
//
//	for _, match := range matches {
//	    relevantDocs = append(relevantDocs, match.Input)
//	}
//
// // Use in chat with relevant context
// err = Ask(ctx, "Explain this code", "copilot-codex", false, relevantDocs)
func GenerateEmbeddings(ctx context.Context, cfg config.Config, inputs []EmbeddingInput, model string) ([]EmbeddingOutput, error) {
	headers, err := getHeaders(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get headers: %w", err)
	}

	threshold := 20000 // Similar to BIG_EMBED_THRESHOLD from Lua
	prepared := prepareEmbeddingRequest(inputs, threshold)

	payload := map[string]any{
		"model": model,
		"input": prepared,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	fmt.Printf("Request payload: %s\n", string(data))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, APIBase+"/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")

	client := getHTTPClient(ctx, cfg)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			fmt.Printf("failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []EmbeddingOutput `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Data, nil
}

// EmbeddingMatch represents a matched document with its similarity score
type EmbeddingMatch struct {
	Input EmbeddingInput
	Score float32
}

// CosineSimilarity calculates the cosine similarity between two embedding vectors
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float32
	for i, val := range a {
		dotProduct += val * b[i]
		normA += val * val
		normB += b[i] * b[i]
	}

	// Early return if either vector is zero
	if normA == 0 || normB == 0 {
		return 0
	}

	// Single conversion to float64 for better performance
	similarity := float32(math.Sqrt(float64(normA * normB)))
	return dotProduct / similarity
}

// FindSimilarDocuments finds the most similar documents to a query embedding
func FindSimilarDocuments(queryEmbedding EmbeddingOutput, documents []EmbeddingInput, documentEmbeddings []EmbeddingOutput, threshold float32) []EmbeddingMatch {
	matches := make([]EmbeddingMatch, 0)

	for i, docEmbedding := range documentEmbeddings {
		score := CosineSimilarity(queryEmbedding.Embedding, docEmbedding.Embedding)
		if score >= threshold {
			matches = append(matches, EmbeddingMatch{
				Input: documents[i],
				Score: score,
			})
		}
	}

	// Sort matches by score in descending order
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	return matches
}
