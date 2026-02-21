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

// OpenAIConfig configures an OpenAI-compatible embedding provider.
type OpenAIConfig struct {
	URL    string // Base URL (e.g., https://api.openai.com/v1)
	APIKey string
	Model  string // Model name (default: text-embedding-3-small)
}

type openaiRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type openaiResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

// OpenAI returns an EmbeddingFunc that calls an OpenAI-compatible API.
func OpenAI(cfg OpenAIConfig) EmbeddingFunc {
	if cfg.Model == "" {
		cfg.Model = "text-embedding-3-small"
	}

	client := &http.Client{Timeout: 30 * time.Second}

	return func(ctx context.Context, text string) ([]float32, error) {
		reqBody, err := json.Marshal(openaiRequest{
			Model: cfg.Model,
			Input: text,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}

		url := cfg.URL + "/embeddings"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("openai request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return nil, fmt.Errorf("openai error (status %d): %s", resp.StatusCode, string(body))
		}

		var result openaiResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		if len(result.Data) == 0 {
			return nil, fmt.Errorf("no embeddings in response")
		}

		// Convert float64 to float32
		embedding := make([]float32, len(result.Data[0].Embedding))
		for i, v := range result.Data[0].Embedding {
			embedding[i] = float32(v)
		}

		return embedding, nil
	}
}
