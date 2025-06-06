package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	// "github.com/cli/go-gh/v2/pkg/api"
)

// For more examples of using go-gh, see:
// https://github.com/cli/go-gh/blob/trunk/example_gh_test.go

// Constants
const (
	APIBase   = "https://api.githubcopilot.com"
	GitHubAPI = "https://api.github.com"
)

// Arguments represents the command-line arguments structure.
type Arguments struct {
	Prompt string
	Model  string
}

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

// parseArgs parses command-line arguments and stdin input.
func parseArgs() (Arguments, error) {
	var model string
	flag.StringVar(&model, "model", "claude-3.7-sonnet", "The AI model to use")
	flag.Parse()

	var prompt string
	if stat, err := os.Stdin.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		// Piped input
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max buffer
		var buf strings.Builder
		for scanner.Scan() {
			buf.WriteString(scanner.Text())
			buf.WriteByte('\n')
		}
		if err := scanner.Err(); err != nil {
			return Arguments{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		prompt = strings.TrimSpace(buf.String())
	} else if flag.NArg() > 0 {
		prompt = flag.Arg(0)
	}

	if prompt == "" {
		return Arguments{}, errors.New("no prompt provided")
	}

	return Arguments{Prompt: prompt, Model: model}, nil
}

// configPath determines the configuration directory for GitHub Copilot.
func configPath() (string, error) {
	// Try XDG config first
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		if isValidDir(xdg) {
			return xdg, nil
		}
	}

	// Windows-specific paths
	if runtime.GOOS == "windows" {
		if path := tryWindowsPaths(); path != "" {
			return path, nil
		}
	}

	// Try user's .config directory
	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}

	configDir := filepath.Join(usr.HomeDir, ".config")
	if isValidDir(configDir) {
		return configDir, nil
	}

	return "", errors.New("no valid config path found")
}

// isValidDir checks if a given path is a valid directory.
func isValidDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// tryWindowsPaths attempts to find the appropriate configuration path on Windows.
func tryWindowsPaths() string {
	// Check environment variables in order of preference
	if path := os.Getenv("LOCALAPPDATA"); isValidDir(path) {
		return path
	}

	if home := os.Getenv("HOME"); home != "" {
		if path := filepath.Join(home, "AppData", "Local"); isValidDir(path) {
			return path
		}
	}

	return ""
}

// readJSONFile reads a JSON file and unmarshals it into the provided variable.
func readJSONFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, v)
}

// Get GitHub token
func getGitHubToken() (string, error) {
	// Check environment variables first
	if token := os.Getenv("GITHUB_TOKEN"); token != "" && os.Getenv("CODESPACES") != "" {
		return token, nil
	}

	configDir, err := configPath()
	if err != nil {
		return "", fmt.Errorf("failed to get config path: %w", err)
	}

	type hostConfig map[string]any

	// Try both config files
	configFiles := []string{
		filepath.Join(configDir, "github-copilot", "hosts.json"),
		filepath.Join(configDir, "github-copilot", "apps.json"),
	}

	for _, path := range configFiles {
		var config hostConfig
		if err := readJSONFile(path, &config); err != nil {
			continue
		}

		// Look for GitHub token in config
		for host, data := range config {
			if !strings.Contains(host, "github.com") {
				continue
			}

			if tokenData, ok := data.(map[string]any); ok {
				if token, ok := tokenData["oauth_token"].(string); ok && token != "" {
					return token, nil
				}
			}
		}
	}

	return "", errors.New("GitHub token not found in environment or config files")
}

// defaultHeaders returns the default headers for the API requests.
func defaultHeaders() map[string]string {
	return map[string]string{
		"Editor-Version":         "Neovim/0.11.2",
		"Editor-Plugin-Version":  "CopilotChat.nvim/*",
		"Copilot-Integration-Id": "vscode-chat",
	}
}

// getHeaders retrieves the authorization headers required for the API requests.
func getHeaders() (map[string]string, error) {
	token, err := getGitHubToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub token: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, GitHubAPI+"/copilot_internal/v2/token", nil)
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
	defer resp.Body.Close()

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

type Message map[string]any
type Options map[string]any

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

// ask sends a chat request to the API and prints the response.
func ask(prompt, model string) error {
	headers, err := getHeaders()
	if err != nil {
		return fmt.Errorf("failed to get headers: %w", err)
	}

	payload := prepareInput(prompt, model)
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, APIBase+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set default headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:       100,
			IdleConnTimeout:    90 * time.Second,
			DisableCompression: false,
			DisableKeepAlives:  false,
			ForceAttemptHTTP2:  true,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReaderSize(resp.Body, 4096)
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)

	var chunk ChatResponse
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
			if content := chunk.Choices[0].Delta.Content; content != "" {
				fmt.Print(content)
			} else if content := chunk.Choices[0].Message.Content; content != "" {
				fmt.Print(content)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading response stream: %w", err)
	}

	return nil
}

// main function to parse arguments and initiate the chat request.
func main() {
	args, err := parseArgs()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	if err := ask(args.Prompt, args.Model); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
