# gh-copilot

A GitHub CLI extension to interact with GitHub Copilot from the command line.

## Features

- Send prompts to GitHub Copilot models directly from the CLI
- Stream responses in real-time with markdown rendering
- Support for predefined prompts via configuration files
- Pipe input from other commands directly to Copilot
- Automatic plain text mode detection for redirected output

## Installation

Install this extension with:

```bash
gh extension install markis/gh-copilot
```

## Usage

```bash
# Basic usage with prompt
gh copilot "Write a bash script to find large files"

# Use a specific model
gh copilot --model claude-3.7-sonnet "Explain quantum computing"

# Use a predefined command from config
gh copilot -c explain "binary search algorithm"

# Pipe content to Copilot
cat error_log.txt | gh copilot "Help me debug this error"

# Force plain text output (disable markdown rendering)
gh copilot --plain "Write a markdown table comparing programming languages"
```

## Configuration

Create a config file at `~/.config/gh-copilot/config.yml` (or `config.yaml`) with predefined prompts:

```yaml
prompts:
  explain: "Please explain this concept in simple terms: "
  debug: "Help me debug this code: "
  summarize: "Summarize the following text: "
```

Then use these commands with:

```bash
gh copilot -c explain "recursion"
```

## Options

- `--model`: Specify the AI model to use (default: "claude-3.7-sonnet")
- `-c`: Use a predefined command from config
- `--plain`: Disable markdown rendering (automatically enabled for redirected output)

## Plain Text Mode

Plain text mode is automatically enabled when:
- Output is being redirected (`gh copilot ... > output.txt`)
- The `NO_COLOR` environment variable is set
- `TERM` environment variable is set to `dumb`
- The `--plain` flag is used

## Requirements

- GitHub CLI (`gh`)
- GitHub Copilot subscription
- GitHub authentication configured
