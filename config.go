package playbookd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/lucas-stellet/playbookd/embed"
)

// Config holds the playbookd configuration loaded from a TOML file.
type Config struct {
	Embedding EmbeddingConfig `toml:"embedding"`
	Data      DataConfig      `toml:"data"`
	Manager   ManagerCfg      `toml:"manager"`
}

// EmbeddingConfig configures the embedding provider.
type EmbeddingConfig struct {
	Provider   string `toml:"provider"` // "google", "openai", "ollama", "noop"
	Mode       string `toml:"mode"`     // "api" or "local"
	Model      string `toml:"model"`
	APIKey     string `toml:"api_key"` // supports ${ENV_VAR} expansion
	URL        string `toml:"url"`
	Dimensions int    `toml:"dimensions"`
}

// DataConfig configures data storage.
type DataConfig struct {
	Dir string `toml:"dir"` // default: "./playbooks"
}

// ManagerCfg configures the PlaybookManager behavior.
type ManagerCfg struct {
	AutoReflect   bool    `toml:"auto_reflect"`
	MaxAge        string  `toml:"max_age"` // duration string like "90d"
	MinConfidence float64 `toml:"min_confidence"`
}

// LoadConfig reads a TOML file at path and returns a parsed Config.
// Environment variables referenced as ${VAR_NAME} in the api_key field are expanded.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.Embedding.APIKey = expandEnvVars(cfg.Embedding.APIKey)

	return &cfg, nil
}

// BuildEmbedFunc constructs an EmbeddingFunc from the embedding configuration.
func (c *Config) BuildEmbedFunc() (embed.EmbeddingFunc, error) {
	switch c.Embedding.Provider {
	case "noop", "":
		return embed.Noop(), nil
	case "openai":
		return embed.OpenAI(embed.OpenAIConfig{
			URL:    c.Embedding.URL,
			APIKey: c.Embedding.APIKey,
			Model:  c.Embedding.Model,
		}), nil
	case "ollama":
		return embed.Ollama(embed.OllamaConfig{
			URL:   c.Embedding.URL,
			Model: c.Embedding.Model,
		}), nil
	case "google":
		return embed.Google(embed.GoogleConfig{
			URL:    c.Embedding.URL,
			APIKey: c.Embedding.APIKey,
			Model:  c.Embedding.Model,
		}), nil
	default:
		return nil, fmt.Errorf("unknown embedding provider: %q", c.Embedding.Provider)
	}
}

// BuildManagerConfig constructs a ManagerConfig from the loaded configuration.
func (c *Config) BuildManagerConfig() (ManagerConfig, error) {
	embedFunc, err := c.BuildEmbedFunc()
	if err != nil {
		return ManagerConfig{}, fmt.Errorf("build embed func: %w", err)
	}

	maxAge, err := parseMaxAge(c.Manager.MaxAge)
	if err != nil {
		return ManagerConfig{}, fmt.Errorf("parse max_age: %w", err)
	}

	dataDir := c.Data.Dir
	if dataDir == "" {
		dataDir = "./playbooks"
	}

	return ManagerConfig{
		DataDir:       dataDir,
		EmbedFunc:     embedFunc,
		EmbedDims:     c.Embedding.Dimensions,
		AutoReflect:   c.Manager.AutoReflect,
		MaxAge:        maxAge,
		MinConfidence: c.Manager.MinConfidence,
	}, nil
}

// expandEnvVars replaces ${VAR_NAME} patterns in s with the corresponding environment variable values.
func expandEnvVars(s string) string {
	return os.Expand(s, func(key string) string {
		// os.Expand handles both $VAR and ${VAR} — we only need to handle the inner key
		return os.Getenv(key)
	})
}

// parseMaxAge parses a duration string in "Nd" format (e.g. "90d") into a time.Duration.
// An empty string returns zero.
func parseMaxAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	if !strings.HasSuffix(s, "d") {
		return 0, fmt.Errorf("unsupported max_age format %q (expected Nd, e.g. \"90d\")", s)
	}

	numStr := strings.TrimSuffix(s, "d")
	days, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid number of days in max_age %q: %w", s, err)
	}

	return time.Duration(days) * 24 * time.Hour, nil
}
