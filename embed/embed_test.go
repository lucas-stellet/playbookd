package embed

import (
	"context"
	"strings"
	"testing"
)

func TestNoop(t *testing.T) {
	fn := Noop()
	if fn == nil {
		t.Fatal("Noop() returned nil EmbeddingFunc")
	}

	ctx := context.Background()
	result, err := fn(ctx, "some text")
	if err != nil {
		t.Errorf("Noop() returned error: %v", err)
	}
	if result != nil {
		t.Errorf("Noop() returned non-nil embedding: %v", result)
	}
}

func TestNoopIgnoresInput(t *testing.T) {
	fn := Noop()
	ctx := context.Background()

	inputs := []string{"", "hello world", "a very long text with lots of content"}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			result, err := fn(ctx, input)
			if err != nil {
				t.Errorf("Noop(%q) returned error: %v", input, err)
			}
			if result != nil {
				t.Errorf("Noop(%q) returned non-nil: %v", input, result)
			}
		})
	}
}

func TestTextForPlaybook(t *testing.T) {
	tests := []struct {
		name        string
		pbName      string
		description string
		tags        []string
		steps       []string
		mustContain []string
		wantEmpty   bool
	}{
		{
			name:        "all fields",
			pbName:      "Deploy Service",
			description: "Procedure to deploy a service",
			tags:        []string{"deploy", "service"},
			steps:       []string{"Build image", "Push to registry"},
			mustContain: []string{"Deploy Service", "Procedure to deploy a service", "deploy", "service", "Build image", "Push to registry"},
		},
		{
			name:      "empty inputs returns empty string",
			pbName:    "",
			wantEmpty: true,
		},
		{
			name:        "name only",
			pbName:      "Only Name",
			mustContain: []string{"Only Name"},
		},
		{
			name:        "tags formatted with prefix",
			pbName:      "Tagged",
			tags:        []string{"alpha", "beta"},
			mustContain: []string{"tags: alpha, beta"},
		},
		{
			name:        "steps formatted with prefix",
			pbName:      "With Steps",
			steps:       []string{"step one", "step two"},
			mustContain: []string{"steps: step one; step two"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TextForPlaybook(tt.pbName, tt.description, tt.tags, tt.steps)

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("TextForPlaybook() = %q, want empty", got)
				}
				return
			}

			for _, must := range tt.mustContain {
				if !strings.Contains(got, must) {
					t.Errorf("TextForPlaybook() = %q, must contain %q", got, must)
				}
			}
		})
	}
}

func TestTextForPlaybookSeparatesWithNewlines(t *testing.T) {
	result := TextForPlaybook("Name", "Desc", []string{"tag"}, []string{"step"})
	// Fields should be joined by newlines.
	if !strings.Contains(result, "\n") {
		t.Errorf("expected newlines between sections, got: %q", result)
	}
}

func TestStepActions(t *testing.T) {
	tests := []struct {
		name  string
		steps []struct{ Action string }
		want  []string
	}{
		{
			name:  "empty list",
			steps: nil,
			want:  []string{},
		},
		{
			name: "all with actions",
			steps: []struct{ Action string }{
				{"do this"},
				{"do that"},
			},
			want: []string{"do this", "do that"},
		},
		{
			name: "skips empty actions",
			steps: []struct{ Action string }{
				{"do this"},
				{""},
				{"do that"},
			},
			want: []string{"do this", "do that"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StepActions(tt.steps)
			if len(got) != len(tt.want) {
				t.Errorf("StepActions() len = %d, want %d", len(got), len(tt.want))
				return
			}
			for i, v := range tt.want {
				if got[i] != v {
					t.Errorf("StepActions()[%d] = %q, want %q", i, got[i], v)
				}
			}
		})
	}
}
