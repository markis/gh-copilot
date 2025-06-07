package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/cli/go-gh/v2/pkg/markdown"
)

// For more examples of using go-gh, see:
// https://github.com/cli/go-gh/blob/trunk/example_gh_test.go

// Constants
const (
	APIBase   = "https://api.githubcopilot.com"
	GitHubAPI = "https://api.github.com"
)

// AuthorizationResponse represents the structure of the response from the GitHub API for authorization.
type AuthorizationResponse struct {
	Token string `json:"token"`
}

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
type (
	Message map[string]any
	Options map[string]any
)

// defaultHeaders returns the default headers for the API requests.
func defaultHeaders() map[string]string {
	return map[string]string{
		"Editor-Version":         "vscode/1.100.2",
		"Copilot-Integration-Id": "vscode-chat",
	}
}

// getHeaders retrieves the authorization headers required for the API requests.
func getHeaders(ctx context.Context) (map[string]string, error) {
	token, err := getGitHubToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub token: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GitHubAPI+"/copilot_internal/v2/token", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	headers := defaultHeaders()
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", "Token "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			fmt.Printf("failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed with status code: %d", resp.StatusCode)
	}

	auth := AuthorizationResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&auth); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if auth.Token == "" {
		return nil, errors.New("received empty token in response")
	}

	headers["Authorization"] = "Bearer " + auth.Token
	return headers, nil
}

// PrepareInput prepares chat input for Copilot API
func prepareInput(prompt, modelID string) map[string]any {
	// Get model configuration
	isOpenAIModel := strings.HasPrefix(modelID, "o1")

	messages := []Message{
		{"role": "user", "content": prompt},
	}

	// Build base request payload with initial capacity
	payload := make(map[string]any, 5) // Pre-allocate for common case
	payload["messages"] = messages
	payload["model"] = modelID

	// Add non-OpenAI specific parameters
	if !isOpenAIModel {
		payload["n"] = 1
		payload["top_p"] = 1
		payload["stream"] = true
	}

	return payload
}

// getHTTPClient returns a singleton HTTP client
var (
	httpClient     *http.Client
	httpClientOnce sync.Once
	defaultTimeout = 60 * time.Second
)

func getHTTPClient(ctx context.Context) *http.Client {
	httpClientOnce.Do(func() {
		transport := &http.Transport{
			MaxIdleConns:       100,
			IdleConnTimeout:    90 * time.Second,
			DisableCompression: false,
			DisableKeepAlives:  false,
			ForceAttemptHTTP2:  true,
		}

		// Add context-aware dial options
		transport.DialContext = (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext

		httpClient = &http.Client{
			Transport: transport,
		}
	})

	// Check if there's a timeout in the context
	if deadline, ok := ctx.Deadline(); ok {
		// Create a clone of the default client with the context timeout
		clientCopy := *httpClient
		clientCopy.Timeout = time.Until(deadline)
		return &clientCopy
	}

	// Return default client with default timeout
	clientCopy := *httpClient
	clientCopy.Timeout = defaultTimeout
	return &clientCopy
}

func Ask(ctx context.Context, prompt, model string, usePlainText bool) error {
	headers, err := getHeaders(ctx)
	if err != nil {
		return fmt.Errorf("failed to get headers: %w", err)
	}

	payload := prepareInput(prompt, model)
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, APIBase+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set default headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	// Use a singleton client instead of creating a new one for each request
	client := getHTTPClient(ctx)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			fmt.Printf("failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return processStream(resp.Body, usePlainText)
}

var (
	markdownRenderer *glamour.TermRenderer
	rendererOnce     sync.Once
)

func getMarkdownRenderer() *glamour.TermRenderer {
	rendererOnce.Do(func() {
		markdownRenderer, _ = glamour.NewTermRenderer(
			markdown.WithTheme("dark"),
			markdown.WithWrap(120),
		)
	})
	return markdownRenderer
}

// findMarkdownBreakPoint looks for natural break points in markdown content
func findMarkdownBreakPoint(content string) int {
	const marker string = "\n\n"
	lastBreak := -1
	idx := strings.LastIndex(content, marker)
	if idx > lastBreak {
		lastBreak = idx + len(marker)
	}

	return lastBreak
}

// processStream handles the streaming response from the API
func processStream(body io.ReadCloser, usePlainText bool) error {
	reader := bufio.NewReaderSize(body, 4096)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // Increase buffer size
	scanner.Split(bufio.ScanLines)

	var buffer strings.Builder
	var chunk ChatResponse
	var lastPrintedLen int

	var renderer *glamour.TermRenderer
	if usePlainText {
		renderer = getMarkdownRenderer()
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line == "data: [DONE]" {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) > 0 {
			content := chunk.Choices[0].Delta.Content
			if content == "" {
				content = chunk.Choices[0].Message.Content
			}
			if content != "" {
				buffer.WriteString(content)
			}
		}

		// Look for common markdown block endings to find natural break points
		content := buffer.String()
		if idx := findMarkdownBreakPoint(content[lastPrintedLen:]); idx > 0 {
			// Process up to the break point
			completeContent := content[:lastPrintedLen+idx]
			section := completeContent[lastPrintedLen:]
			if renderer == nil {
				fmt.Print(section)
			} else {
				section = strings.TrimSpace(section)
				if strings.HasPrefix(section, "#") {
					fmt.Println() // Print a newline before headings
				}
				mdContent, err := renderer.Render(section)
				if err != nil {
					return fmt.Errorf("failed to render markdown: %w", err)
				}
				fmt.Println(strings.TrimSpace(mdContent))
			}
			lastPrintedLen = len(completeContent)
		}
	}

	// Process any remaining content
	if remaining := buffer.String()[lastPrintedLen:]; remaining != "" {
		if renderer == nil {
			fmt.Print(remaining)
		} else {
			if strings.HasPrefix(remaining, "#") {
				fmt.Println() // Print a newline before headings
			}
			mdContent, err := renderer.Render(strings.TrimSpace(remaining))
			if err != nil {
				return fmt.Errorf("failed to render markdown: %w", err)
			}
			fmt.Println(strings.TrimSpace(mdContent))
		}
	}
	fmt.Println()

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading response stream: %w", err)
	}

	return nil
}
