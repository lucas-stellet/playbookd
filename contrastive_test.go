package playbookd

import (
	"context"
	"fmt"
	"testing"
)

// samplePlaybookWithStats returns a playbook with pre-set success/failure counts
// and recalculated stats (success rate and Wilson confidence).
func samplePlaybookWithStats(name string, successes, failures int) *Playbook {
	pb := samplePlaybook(name)
	pb.SuccessCount = successes
	pb.FailureCount = failures
	pb.UpdateStats()
	return pb
}

// setupContrastivePlaybooks creates three deployment playbooks with varying
// confidence levels and returns the manager. Each playbook is created, then
// its stats are manually set, saved, and re-indexed.
func setupContrastivePlaybooks(t *testing.T) *PlaybookManager {
	t.Helper()
	pm := newTestManager(t)
	ctx := context.Background()

	type spec struct {
		name      string
		successes int
		failures  int
	}

	specs := []spec{
		{"Proven Deployment", 9, 1},   // high confidence ~0.60
		{"Failed Deployment", 1, 9},   // low confidence ~0.02
		{"Mixed Deployment", 7, 3},    // medium confidence ~0.40 (neutral zone: 0.3–0.5)
	}

	for _, s := range specs {
		pb := samplePlaybookWithStats(s.name, 0, 0) // stats will be set after Create
		pb.Description = "A deployment procedure for testing contrastive search"
		pb.Tags = []string{"deployment", "test"}
		if err := pm.Create(ctx, pb); err != nil {
			t.Fatalf("Create %s: %v", s.name, err)
		}

		got, err := pm.Get(ctx, pb.ID)
		if err != nil {
			t.Fatalf("Get %s: %v", s.name, err)
		}
		got.SuccessCount = s.successes
		got.FailureCount = s.failures
		got.UpdateStats()
		if err := pm.store.SavePlaybook(ctx, got); err != nil {
			t.Fatalf("SavePlaybook %s: %v", s.name, err)
		}
		if err := pm.indexer.Index(ctx, got); err != nil {
			t.Fatalf("Index %s: %v", s.name, err)
		}
	}

	return pm
}

func TestSearchWithContext(t *testing.T) {
	pm := setupContrastivePlaybooks(t)
	ctx := context.Background()

	cr, err := pm.SearchWithContext(ctx, ContrastiveQuery{
		SearchQuery: SearchQuery{
			Text: "deployment",
			Mode: SearchModeBM25,
		},
	})
	if err != nil {
		t.Fatalf("SearchWithContext: %v", err)
	}

	if len(cr.Positive) < 1 {
		t.Errorf("expected at least 1 positive result, got %d", len(cr.Positive))
	}
	if len(cr.Negative) < 1 {
		t.Errorf("expected at least 1 negative result, got %d", len(cr.Negative))
	}
	if cr.Query != "deployment" {
		t.Errorf("Query = %q, want %q", cr.Query, "deployment")
	}
}

func TestSearchWithContextIncludeNeutral(t *testing.T) {
	pm := setupContrastivePlaybooks(t)
	ctx := context.Background()

	cr, err := pm.SearchWithContext(ctx, ContrastiveQuery{
		SearchQuery: SearchQuery{
			Text: "deployment",
			Mode: SearchModeBM25,
		},
		IncludeNeutral: true,
	})
	if err != nil {
		t.Fatalf("SearchWithContext: %v", err)
	}

	if len(cr.Neutral) < 1 {
		t.Errorf("expected at least 1 neutral result, got %d", len(cr.Neutral))
	}
}

func TestSearchWithContextCustomThresholds(t *testing.T) {
	pm := setupContrastivePlaybooks(t)
	ctx := context.Background()

	cr, err := pm.SearchWithContext(ctx, ContrastiveQuery{
		SearchQuery: SearchQuery{
			Text: "deployment",
			Mode: SearchModeBM25,
		},
		PositiveMinConfidence: 0.99,
	})
	if err != nil {
		t.Fatalf("SearchWithContext: %v", err)
	}

	if len(cr.Positive) != 0 {
		t.Errorf("expected 0 positive results with 0.99 threshold, got %d", len(cr.Positive))
	}
}

func TestSearchWithContextNoResults(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	cr, err := pm.SearchWithContext(ctx, ContrastiveQuery{
		SearchQuery: SearchQuery{
			Text: "nonexistent-xyzzy-query",
			Mode: SearchModeBM25,
		},
	})
	if err != nil {
		t.Fatalf("SearchWithContext: %v", err)
	}

	if len(cr.Positive) != 0 {
		t.Errorf("expected 0 positive results, got %d", len(cr.Positive))
	}
	if len(cr.Negative) != 0 {
		t.Errorf("expected 0 negative results, got %d", len(cr.Negative))
	}
	if len(cr.Neutral) != 0 {
		t.Errorf("expected 0 neutral results, got %d", len(cr.Neutral))
	}
}

func TestSearchWithContextLimitsCapping(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	// Create 10 high-confidence playbooks with the same keyword.
	for i := 0; i < 10; i++ {
		pb := samplePlaybookWithStats(fmt.Sprintf("Scaling Procedure %d", i), 0, 0)
		pb.Description = "A scaling procedure for testing limit capping"
		pb.Tags = []string{"scaling", "test"}
		if err := pm.Create(ctx, pb); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}

		got, err := pm.Get(ctx, pb.ID)
		if err != nil {
			t.Fatalf("Get %d: %v", i, err)
		}
		got.SuccessCount = 9
		got.FailureCount = 0
		got.UpdateStats()
		if err := pm.store.SavePlaybook(ctx, got); err != nil {
			t.Fatalf("SavePlaybook %d: %v", i, err)
		}
		if err := pm.indexer.Index(ctx, got); err != nil {
			t.Fatalf("Index %d: %v", i, err)
		}
	}

	cr, err := pm.SearchWithContext(ctx, ContrastiveQuery{
		SearchQuery: SearchQuery{
			Text:  "scaling",
			Mode:  SearchModeBM25,
			Limit: 3,
		},
	})
	if err != nil {
		t.Fatalf("SearchWithContext: %v", err)
	}

	if len(cr.Positive) > 3 {
		t.Errorf("expected at most 3 positive results, got %d", len(cr.Positive))
	}
}
