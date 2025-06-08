package client

import (
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

	"github.com/markis/gh-copilot/internal/render"
	"github.com/markis/gh-copilot/internal/stream"
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

// Ask sends a chat request to the Copilot API and processes the response.
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

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := getHTTPClient(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	parser := stream.NewParser()
	renderer := render.NewTerminalRenderer(usePlainText)

	go parser.Process(resp.Body)
	return renderer.Render(parser.Chunks())
}
