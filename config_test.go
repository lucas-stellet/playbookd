package playbookd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	content := `
[embedding]
provider = "openai"
mode = "api"
model = "text-embedding-3-small"
api_key = "sk-test"
url = "https://api.openai.com/v1"
dimensions = 1536

[data]
dir = "/tmp/playbooks"

[manager]
auto_reflect = true
max_age = "90d"
min_confidence = 0.4
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Embedding.Provider != "openai" {
		t.Errorf("Provider = %q, want %q", cfg.Embedding.Provider, "openai")
	}
	if cfg.Embedding.Model != "text-embedding-3-small" {
		t.Errorf("Model = %q, want %q", cfg.Embedding.Model, "text-embedding-3-small")
	}
	if cfg.Embedding.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want %q", cfg.Embedding.APIKey, "sk-test")
	}
	if cfg.Embedding.URL != "https://api.openai.com/v1" {
		t.Errorf("URL = %q, want %q", cfg.Embedding.URL, "https://api.openai.com/v1")
	}
	if cfg.Embedding.Dimensions != 1536 {
		t.Errorf("Dimensions = %d, want %d", cfg.Embedding.Dimensions, 1536)
	}
	if cfg.Data.Dir != "/tmp/playbooks" {
		t.Errorf("Data.Dir = %q, want %q", cfg.Data.Dir, "/tmp/playbooks")
	}
	if !cfg.Manager.AutoReflect {
		t.Error("Manager.AutoReflect = false, want true")
	}
	if cfg.Manager.MaxAge != "90d" {
		t.Errorf("Manager.MaxAge = %q, want %q", cfg.Manager.MaxAge, "90d")
	}
	if cfg.Manager.MinConfidence != 0.4 {
		t.Errorf("Manager.MinConfidence = %f, want %f", cfg.Manager.MinConfidence, 0.4)
	}
}

func TestLoadConfigEnvExpansion(t *testing.T) {
	t.Setenv("TEST_API_KEY", "secret-from-env")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	content := `
[embedding]
provider = "openai"
api_key = "${TEST_API_KEY}"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Embedding.APIKey != "secret-from-env" {
		t.Errorf("APIKey = %q, want %q", cfg.Embedding.APIKey, "secret-from-env")
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.toml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestBuildManagerConfig(t *testing.T) {
	cfg := &Config{
		Embedding: EmbeddingConfig{
			Provider:   "noop",
			Dimensions: 512,
		},
		Data: DataConfig{
			Dir: "/data/playbooks",
		},
		Manager: ManagerCfg{
			AutoReflect:   true,
			MaxAge:        "30d",
			MinConfidence: 0.5,
		},
	}

	mc, err := cfg.BuildManagerConfig()
	if err != nil {
		t.Fatalf("BuildManagerConfig: %v", err)
	}

	if mc.DataDir != "/data/playbooks" {
		t.Errorf("DataDir = %q, want %q", mc.DataDir, "/data/playbooks")
	}
	if mc.EmbedDims != 512 {
		t.Errorf("EmbedDims = %d, want %d", mc.EmbedDims, 512)
	}
	if !mc.AutoReflect {
		t.Error("AutoReflect = false, want true")
	}
	want := 30 * 24 * time.Hour
	if mc.MaxAge != want {
		t.Errorf("MaxAge = %v, want %v", mc.MaxAge, want)
	}
	if mc.MinConfidence != 0.5 {
		t.Errorf("MinConfidence = %f, want %f", mc.MinConfidence, 0.5)
	}
	if mc.EmbedFunc == nil {
		t.Error("EmbedFunc = nil, want non-nil")
	}
}

func TestBuildManagerConfigDefaultDir(t *testing.T) {
	cfg := &Config{
		Embedding: EmbeddingConfig{Provider: "noop"},
	}

	mc, err := cfg.BuildManagerConfig()
	if err != nil {
		t.Fatalf("BuildManagerConfig: %v", err)
	}

	if mc.DataDir != "./playbooks" {
		t.Errorf("DataDir = %q, want %q", mc.DataDir, "./playbooks")
	}
}

func TestBuildEmbedFunc(t *testing.T) {
	tests := []struct {
		provider string
		wantErr  bool
	}{
		{"noop", false},
		{"", false},
		{"openai", false},
		{"ollama", false},
		{"google", false},
		{"unknown-provider", true},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			cfg := &Config{
				Embedding: EmbeddingConfig{Provider: tt.provider},
			}
			fn, err := cfg.BuildEmbedFunc()
			if tt.wantErr {
				if err == nil {
					t.Errorf("provider %q: expected error, got nil", tt.provider)
				}
			} else {
				if err != nil {
					t.Errorf("provider %q: unexpected error: %v", tt.provider, err)
				}
				if fn == nil {
					t.Errorf("provider %q: EmbedFunc is nil", tt.provider)
				}
			}
		})
	}
}

func TestParseMaxAge(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"90d", 90 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"0d", 0, false},
		{"", 0, false},
		{"30h", 0, true},
		{"abc", 0, true},
		{"d", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseMaxAge(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseMaxAge(%q): expected error, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("parseMaxAge(%q): unexpected error: %v", tt.input, err)
				}
				if got != tt.want {
					t.Errorf("parseMaxAge(%q) = %v, want %v", tt.input, got, tt.want)
				}
			}
		})
	}
}
