// Package embed provides embedding functions for generating vector
// representations of text, with support for Ollama and OpenAI providers.
package embed

import (
	"context"
	"fmt"
	"strings"
)

// EmbeddingFunc generates a vector embedding from text.
type EmbeddingFunc func(ctx context.Context, text string) ([]float32, error)

// Noop returns an EmbeddingFunc that always returns nil (BM25-only mode).
func Noop() EmbeddingFunc {
	return func(_ context.Context, _ string) ([]float32, error) {
		return nil, nil
	}
}

// TextForPlaybook concatenates playbook fields into a single string for embedding.
// This ensures consistent text representation across indexing and search.
func TextForPlaybook(name, description string, tags []string, steps []string) string {
	var parts []string

	if name != "" {
		parts = append(parts, name)
	}
	if description != "" {
		parts = append(parts, description)
	}
	if len(tags) > 0 {
		parts = append(parts, fmt.Sprintf("tags: %s", strings.Join(tags, ", ")))
	}
	if len(steps) > 0 {
		parts = append(parts, fmt.Sprintf("steps: %s", strings.Join(steps, "; ")))
	}

	return strings.Join(parts, "\n")
}

// StepActions extracts action descriptions from a list of steps for text generation.
func StepActions(steps []struct{ Action string }) []string {
	actions := make([]string, 0, len(steps))
	for _, s := range steps {
		if s.Action != "" {
			actions = append(actions, s.Action)
		}
	}
	return actions
}
