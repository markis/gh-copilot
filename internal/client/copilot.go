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

	"github.com/markis/gh-copilot/internal/args"
	"github.com/markis/gh-copilot/internal/config"
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

// ApiResponse represents the structure of the response from the chat API.
type ApiResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Index int `json:"index"`
	} `json:"choices"`
}

type Role string

const (
	UserRole      Role = "user"
	SystemRole    Role = "system"
	AssistantRole Role = "assistant"
)

type Message struct {
	Role    Role   `json:"role"`    // "user", "assistant", or "system"
	Content string `json:"content"` // The message content
}

type ApiPayload struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	NumOfResponses int       `json:"n,omitempty"`      // Number of responses to generate
	TopP           float64   `json:"top_p,omitempty"`  // Top-p sampling
	Stream         bool      `json:"stream,omitempty"` // Whether to stream the response
}

// defaultHeaders returns the default headers for the API requests.
func defaultHeaders() map[string]string {
	return map[string]string{
		"Editor-Version":         "vscode/*",
		"Copilot-Integration-Id": "vscode-chat",
	}
}

// getHeaders retrieves the authorization headers required for the API requests.
func getHeaders(ctx context.Context, cfg config.Config) (map[string]string, error) {
	token, err := getGitHubToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub token: %w", err)
	}

	client := getHTTPClient(ctx, cfg)
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

// prepareInput constructs the API payload from user arguments.
// It converts user prompts into the message format expected by the API,
// sets the appropriate model, and configures model-specific parameters.
func prepareInput(args args.Arguments) ApiPayload {
	// Get model configuration
	isOpenAIModel := strings.HasPrefix(args.Model, "o1")

	messages := make([]Message, 0, len(args.Prompts))
	for _, prompt := range args.Prompts {
		if strings.TrimSpace(prompt) == "" {
			continue // Skip empty prompts
		}

		messages = append(messages, Message{
			Role:    UserRole,
			Content: prompt,
		})
	}

	// Build base request payload with initial capacity
	payload := ApiPayload{
		Model:    args.Model,
		Messages: messages,
	}

	// Add non-OpenAI specific parameters
	if !isOpenAIModel {
		payload.NumOfResponses = 1
		payload.TopP = 1.0
		payload.Stream = true
	}

	return payload
}

// getHTTPClient returns a singleton HTTP client
var (
	httpClient     *http.Client
	httpClientOnce sync.Once
)

func getHTTPClient(ctx context.Context, cfg config.Config) *http.Client {
	httpClientOnce.Do(func() {
		transport := &http.Transport{
			MaxIdleConns:       cfg.Http.MaxIdleConns,
			IdleConnTimeout:    cfg.Http.IdleConnTimeout,
			DisableCompression: cfg.Http.DisableCompression,
			DisableKeepAlives:  cfg.Http.DisableKeepAlives,
			ForceAttemptHTTP2:  cfg.Http.ForceAttemptHTTP2,
		}

		// Add context-aware dial options
		transport.DialContext = (&net.Dialer{
			Timeout:   cfg.Http.DialContextTimeout,
			KeepAlive: cfg.Http.DialContextKeepAlive,
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
	clientCopy.Timeout = cfg.Http.HttpClientTimeout
	return &clientCopy
}

// Ask sends a chat request to the Copilot API and processes the response.
func Ask(ctx context.Context, cfg config.Config, args args.Arguments) error {
	headers, err := getHeaders(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to get headers: %w", err)
	}

	payload := prepareInput(args)
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

	client := getHTTPClient(ctx, cfg)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	parser := stream.NewParser(ctx)
	renderer, err := render.NewTerminalRenderer(ctx, cfg, args)
	if err != nil {
		return fmt.Errorf("failed to create renderer: %w", err)
	}

	go parser.Process(resp.Body)
	return renderer.Render(parser.Chunks())
}
