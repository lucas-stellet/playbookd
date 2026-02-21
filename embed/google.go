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

// GoogleConfig configures the Google Gemini embedding provider.
type GoogleConfig struct {
	URL    string // Base URL (default: https://generativelanguage.googleapis.com/v1beta)
	APIKey string // API key
	Model  string // Model name (default: gemini-embedding-001)
}

type googleRequestPart struct {
	Text string `json:"text"`
}

type googleRequestContent struct {
	Parts []googleRequestPart `json:"parts"`
}

type googleRequest struct {
	Content googleRequestContent `json:"content"`
}

type googleResponseEmbedding struct {
	Values []float64 `json:"values"`
}

type googleResponse struct {
	Embedding googleResponseEmbedding `json:"embedding"`
}

// Google returns an EmbeddingFunc that calls the Google Gemini embedContent API.
func Google(cfg GoogleConfig) EmbeddingFunc {
	if cfg.URL == "" {
		cfg.URL = "https://generativelanguage.googleapis.com/v1beta"
	}
	if cfg.Model == "" {
		cfg.Model = "gemini-embedding-001"
	}

	client := &http.Client{Timeout: 30 * time.Second}

	return func(ctx context.Context, text string) ([]float32, error) {
		reqBody, err := json.Marshal(googleRequest{
			Content: googleRequestContent{
				Parts: []googleRequestPart{{Text: text}},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}

		url := cfg.URL + "/models/" + cfg.Model + ":embedContent?key=" + cfg.APIKey
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("google request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return nil, fmt.Errorf("google error (status %d): %s", resp.StatusCode, string(body))
		}

		var result googleResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		if len(result.Embedding.Values) == 0 {
			return nil, fmt.Errorf("no embeddings in response")
		}

		// Convert float64 to float32
		embedding := make([]float32, len(result.Embedding.Values))
		for i, v := range result.Embedding.Values {
			embedding[i] = float32(v)
		}

		return embedding, nil
	}
}
