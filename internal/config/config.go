package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/creasty/defaults"
	"gopkg.in/yaml.v3"
)

const (
	configLoadTimeout = 10 * time.Second
	configDirName     = "gh-copilot"
	defaultConfig     = ".config"
)

var configFiles = []string{
	"config.yaml",
	"config.yml",
}

// Config represents the structure of the configuration file used by the application.
type Config struct {
	ContextTimeout time.Duration `yaml:"context_timeout,omitempty" default:"10m"`
	Model          string        `yaml:"model" default:"claude-3.7-sonnet"`

	Http    ConfigHttp   `yaml:"http"`
	Render  ConfigRender `yaml:"render"`
	Prompts Prompts      `yaml:"prompts"`
}

type Prompts map[string]ConfigPrompt

type ConfigPrompt struct {
	Model  string `yaml:"model,omitempty"`
	Prompt string `yaml:"prompt"`
}

type ConfigHttp struct {
	IdleConnTimeout      time.Duration `yaml:"idle_conn_timeout,omitempty" default:"90s"`
	DialContextTimeout   time.Duration `yaml:"dial_context_timeout,omitempty" default:"30s"`
	DialContextKeepAlive time.Duration `yaml:"dial_context_keep_alive,omitempty" default:"30s"`
	HttpClientTimeout    time.Duration `yaml:"http_client_timeout,omitempty" default:"60s"`
	MaxIdleConns         int           `yaml:"max_idle_conns,omitempty" default:"100"`
	DisableCompression   bool          `yaml:"disable_compression,omitempty" default:"false"`
	DisableKeepAlives    bool          `yaml:"disable_keep_alives,omitempty" default:"false"`
	ForceAttemptHTTP2    bool          `yaml:"force_attempt_http2,omitempty" default:"true"`
}

// ConfigRender defines how the output should be formatted and displayed.
type ConfigRender struct {
	Format    string `yaml:"format,omitempty" default:"markdown"` // "markdown" or "plain"
	Theme     string `yaml:"theme,omitempty" default:"auto"`      // glamour theme name, "auto" for auto-detect
	WrapLines bool   `yaml:"wrap_lines,omitempty" default:"true"`
	WrapWidth int    `yaml:"wrap_width,omitempty" default:"120"`
}

// configResult is a struct used to return the configuration and any error that occurs during loading.
type configResult struct {
	config *Config
	err    error
}

var defaultPrompts = Prompts{
	"ask": {Prompt: "Answer the following question."},
}

// newDefaultConfig creates a new default configuration with an empty prompts map.
func newDefaultConfig() *Config {
	return &Config{
		Prompts: defaultPrompts,
	}
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
	if err := defaults.Set(cfg); err != nil {
		return nil, fmt.Errorf("setting defaults: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

// LoadConfig loads the configuration from the user's home directory, with a timeout.
func LoadConfig(ctx context.Context) (Config, error) {
	ctx, cancel := context.WithTimeout(ctx, configLoadTimeout)
	defer cancel()

	result := make(chan configResult, 1)

	go func() {
		cfg, err := loadConfigFiles(ctx)
		result <- configResult{config: cfg, err: err}
	}()

	done := ctx.Done()
	select {
	case <-done:
		return Config{}, ctx.Err()
	case r := <-result:
		if r.config == nil {
			return Config{}, r.err
		}
		return *r.config, r.err
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
