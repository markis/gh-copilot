package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Prompts map[string]string `yaml:"prompts"`
}

func loadConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(home, ".config", "gh-copilot")
	var data []byte

	// Try to read the first available config file
	for _, filename := range []string{"config.yaml", "config.yml"} {
		data, err = os.ReadFile(filepath.Join(configDir, filename))
		if err == nil {
			break // Successfully read the file
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read config file %s: %w", filename, err)
		}
	}

	// Return default config if no config file exists
	if data == nil {
		return &Config{
			Prompts: make(map[string]string),
		}, nil
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}
