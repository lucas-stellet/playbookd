package main

import (
	"flag"
	"fmt"
	"os"
)

// runInit generates a .playbookd.toml configuration template in the current directory.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	force := fs.Bool("force", false, "overwrite existing .playbookd.toml")
	provider := fs.String("provider", "noop", "embedding provider: noop, openai, ollama, google")
	if err := fs.Parse(args); err != nil {
		return err
	}

	const configFile = ".playbookd.toml"

	if !*force {
		if _, err := os.Stat(configFile); err == nil {
			return fmt.Errorf("`%s` already exists (use -force to overwrite)", configFile)
		}
	}

	content := buildTemplate(*provider)

	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	fmt.Println("Created .playbookd.toml")
	fmt.Println("Next: edit the file to configure your embedding provider, then run your agent.")
	return nil
}

// buildTemplate returns the TOML configuration template for the given provider.
func buildTemplate(provider string) string {
	header := `# playbookd configuration
# See https://github.com/lucas-stellet/playbookd for documentation.

`

	var embedding string
	switch provider {
	case "google":
		embedding = `[embedding]
# Embedding provider: "noop", "openai", "ollama", "google"
provider = "google"
# mode = "api"         # "api" or "local"
model = "gemini-embedding-001"
api_key = "${GOOGLE_API_KEY}"
url = "https://generativelanguage.googleapis.com/v1beta"
dimensions = 768
`
	case "openai":
		embedding = `[embedding]
# Embedding provider: "noop", "openai", "ollama", "google"
provider = "openai"
# mode = "api"         # "api" or "local"
model = "text-embedding-3-small"
api_key = "${OPENAI_API_KEY}"
url = "https://api.openai.com/v1"
dimensions = 1536
`
	case "ollama":
		embedding = `[embedding]
# Embedding provider: "noop", "openai", "ollama", "google"
provider = "ollama"
mode = "local"
model = "nomic-embed-text-v2-moe"
# api_key = ""
url = "http://localhost:11434"
dimensions = 384
`
	default: // noop
		embedding = `[embedding]
# Embedding provider: "noop", "openai", "ollama", "google"
provider = "noop"
# mode = "api"         # "api" or "local"
# model = ""
# api_key = "${OPENAI_API_KEY}"
# url = ""
# dimensions = 0
`
	}

	rest := `
[data]
dir = "./playbooks"

[manager]
auto_reflect = false
max_age = "90d"
min_confidence = 0.3
`

	return header + embedding + rest
}
