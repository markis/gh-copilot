package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

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

// getGitHubToken retrieves the GitHub token from environment variables or config files
func getGitHubToken() (string, error) {
	// Check environment variables first - fast path
	if token := os.Getenv("GITHUB_TOKEN"); token != "" && os.Getenv("CODESPACES") != "" {
		return token, nil
	}

	configDir, err := configPath()
	if err != nil {
		return "", fmt.Errorf("failed to get config path: %w", err)
	}

	configFiles := []string{
		filepath.Join(configDir, "github-copilot", "hosts.json"),
		filepath.Join(configDir, "github-copilot", "apps.json"),
	}

	for _, path := range configFiles {
		var config map[string]any
		if err := readJSONFile(path, &config); err != nil {
			continue
		}

		if token := extractGitHubToken(config); token != "" {
			return token, nil
		}
	}

	return "", errors.New("GitHub token not found in environment or config files")
}

// extractGitHubToken helps extract the token from config data
func extractGitHubToken(config map[string]any) string {
	for host, data := range config {
		if !strings.Contains(host, "github.com") {
			continue
		}

		tokenData, ok := data.(map[string]any)
		if !ok {
			continue
		}

		if token, ok := tokenData["oauth_token"].(string); ok && token != "" {
			return token
		}
	}
	return ""
}
