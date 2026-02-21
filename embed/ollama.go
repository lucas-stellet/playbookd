package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaConfig configures the Ollama embedding provider.
type OllamaConfig struct {
	URL   string // Base URL (default: http://localhost:11434)
	Model string // Model name (default: nomic-embed-text-v2-moe)
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaResponse struct {
	Embedding []float64 `json:"embedding"`
}

// Ollama returns an EmbeddingFunc that calls the Ollama API.
func Ollama(cfg OllamaConfig) EmbeddingFunc {
	if cfg.URL == "" {
		cfg.URL = "http://localhost:11434"
	}
	if cfg.Model == "" {
		cfg.Model = "nomic-embed-text-v2-moe"
	}

	client := &http.Client{Timeout: 30 * time.Second}

	return func(ctx context.Context, text string) ([]float32, error) {
		reqBody, err := json.Marshal(ollamaRequest{
			Model:  cfg.Model,
			Prompt: text,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}

		url := cfg.URL + "/api/embeddings"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("ollama request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return nil, fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, string(body))
		}

		var result ollamaResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		// Convert float64 to float32
		embedding := make([]float32, len(result.Embedding))
		for i, v := range result.Embedding {
			embedding[i] = float32(v)
		}

		return embedding, nil
	}
}
