package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	configDirName = "gh-copilot"
	defaultConfig = ".config"
)

var configFiles = []string{
	"config.yaml",
	"config.yml",
}

// Config represents the structure of the configuration file used by the application.
type Config struct {
	Model   string            `yaml:"model"`
	Prompts map[string]string `yaml:"prompts"`
}

// configResult is a struct used to return the configuration and any error that occurs during loading.
type configResult struct {
	config *Config
	err    error
}

// newDefaultConfig creates a new default configuration with an empty prompts map.
func newDefaultConfig() *Config {
	return &Config{Prompts: map[string]string{}}
}

// getConfigPath retrieves the path to the configuration directory based on the XDG_CONFIG_HOME environment variable.
func getConfigPath() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		configHome = filepath.Join(home, defaultConfig)
	}

	return filepath.Join(configHome, configDirName), nil
}

// tryLoadConfig attempts to load a configuration file from the specified path.
func tryLoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := newDefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

// LoadConfig loads the configuration from the user's home directory, with a timeout.
func LoadConfig(ctx context.Context) (*Config, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result := make(chan configResult, 1)

	go func() {
		cfg, err := loadConfigFiles(ctx)
		result <- configResult{config: cfg, err: err}
	}()

	done := ctx.Done()
	select {
	case <-done:
		return nil, ctx.Err()
	case r := <-result:
		return r.config, r.err
	}
}

// loadConfigFiles loads configuration files from the user's home directory.
func loadConfigFiles(ctx context.Context) (*Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context error before loading config: %w", err)
	}

	configDir, err := getConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get config path: %w", err)
	}

	// Return default config early if directory doesn't exist
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		return newDefaultConfig(), nil
	}

	for _, filename := range configFiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		cfg, err := tryLoadConfig(filepath.Join(configDir, filename))
		if err == nil {
			return cfg, nil
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load config from %s: %w", filename, err)
		}
	}

	return newDefaultConfig(), nil
}
