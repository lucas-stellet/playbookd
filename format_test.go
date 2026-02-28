package playbookd

import (
	"strings"
	"testing"
)

func TestFormatForContextPositiveOnly(t *testing.T) {
	cr := &ContrastiveResults{
		Query: "deploy app",
		Positive: []SearchResult{
			{
				Playbook: &Playbook{
					Name:         "Safe Deploy",
					Confidence:   0.85,
					SuccessCount: 17,
					FailureCount: 3,
					Steps: []Step{
						{Order: 1, Action: "Run tests"},
						{Order: 2, Action: "Deploy to staging"},
					},
					Lessons: []Lesson{
						{Content: "Always run smoke tests after deploy"},
					},
				},
			},
		},
	}

	out := FormatForContext(cr)

	if !strings.Contains(out, "Proven Approaches") {
		t.Error("expected 'Proven Approaches' header")
	}
	if !strings.Contains(out, "Safe Deploy") {
		t.Error("expected playbook name")
	}
	if !strings.Contains(out, "85%") {
		t.Error("expected confidence percentage")
	}
	if !strings.Contains(out, "Run tests") {
		t.Error("expected step action")
	}
	if !strings.Contains(out, "smoke tests") {
		t.Error("expected lesson content")
	}
	if strings.Contains(out, "Failed Approaches") {
		t.Error("should not contain 'Failed Approaches' when only positives exist")
	}
}

func TestFormatForContextNegativeOnly(t *testing.T) {
	cr := &ContrastiveResults{
		Query: "deploy app",
		Negative: []SearchResult{
			{
				Playbook: &Playbook{
					Name:         "Risky Deploy",
					Confidence:   0.05,
					SuccessCount: 1,
					FailureCount: 9,
					Lessons: []Lesson{
						{Content: "Skipping tests caused rollback"},
					},
				},
			},
		},
	}

	out := FormatForContext(cr)

	if !strings.Contains(out, "Failed Approaches") {
		t.Error("expected 'Failed Approaches' header")
	}
	if !strings.Contains(out, "Risky Deploy") {
		t.Error("expected playbook name")
	}
	if !strings.Contains(out, "failure rate: 90%") {
		t.Error("expected failure rate percentage")
	}
	if !strings.Contains(out, "What failed") {
		t.Error("expected 'What failed' section")
	}
	if !strings.Contains(out, "Skipping tests") {
		t.Error("expected lesson content")
	}
	if strings.Contains(out, "Proven Approaches") {
		t.Error("should not contain 'Proven Approaches' when only negatives exist")
	}
}

func TestFormatForContextBothPositiveAndNegative(t *testing.T) {
	cr := &ContrastiveResults{
		Query: "deploy",
		Positive: []SearchResult{
			{Playbook: &Playbook{Name: "Good", Confidence: 0.8, SuccessCount: 8, FailureCount: 2}},
		},
		Negative: []SearchResult{
			{Playbook: &Playbook{Name: "Bad", Confidence: 0.1, SuccessCount: 1, FailureCount: 9}},
		},
	}

	out := FormatForContext(cr)

	if !strings.Contains(out, "Proven Approaches") {
		t.Error("expected 'Proven Approaches'")
	}
	if !strings.Contains(out, "Failed Approaches") {
		t.Error("expected 'Failed Approaches'")
	}
	if !strings.Contains(out, "Follow the proven approaches") {
		t.Error("expected closing instruction")
	}
}

func TestFormatForContextEmpty(t *testing.T) {
	cr := &ContrastiveResults{Query: "nothing"}
	out := FormatForContext(cr)

	if !strings.Contains(out, "No relevant playbooks found") {
		t.Errorf("expected 'No relevant playbooks found', got: %q", out)
	}
}

func TestFormatForContextNil(t *testing.T) {
	out := FormatForContext(nil)
	if out != "" {
		t.Errorf("expected empty string for nil input, got: %q", out)
	}
}
