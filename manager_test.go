package playbookd

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lucas-stellet/playbookd/embed"
)

// newTestManager creates a PlaybookManager using a temp directory and the Noop embedder.
func newTestManager(t *testing.T) *PlaybookManager {
	t.Helper()
	dir := t.TempDir()
	pm, err := NewPlaybookManager(ManagerConfig{
		DataDir:   dir,
		EmbedFunc: embed.Noop(),
		EmbedDims: 0,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewPlaybookManager: %v", err)
	}
	t.Cleanup(func() { pm.Close() })
	return pm
}

// samplePlaybook returns a minimal valid playbook for tests.
func samplePlaybook(name string) *Playbook {
	return &Playbook{
		Name:        name,
		Description: "A playbook for testing: " + name,
		Tags:        []string{"test"},
		Category:    "qa",
		Steps: []Step{
			{Order: 1, Action: "Check preconditions"},
			{Order: 2, Action: "Execute main logic"},
		},
	}
}

func TestManagerCreate(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	pb := samplePlaybook("Create Test")
	if err := pm.Create(ctx, pb); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if pb.ID == "" {
		t.Error("expected ID to be set after Create")
	}
	if pb.Slug == "" {
		t.Error("expected Slug to be set after Create")
	}
	if pb.Version != 1 {
		t.Errorf("Version = %d, want 1", pb.Version)
	}
}

func TestManagerGet(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	pb := samplePlaybook("Get Test")
	if err := pm.Create(ctx, pb); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := pm.Get(ctx, pb.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != pb.Name {
		t.Errorf("Name = %q, want %q", got.Name, pb.Name)
	}
}

func TestManagerGetNotFound(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	_, err := pm.Get(ctx, "nonexistent-id")
	if err == nil {
		t.Error("expected error for missing playbook")
	}
}

func TestManagerList(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	if err := pm.Create(ctx, samplePlaybook("List Alpha")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := pm.Create(ctx, samplePlaybook("List Beta")); err != nil {
		t.Fatalf("setup: %v", err)
	}

	results, err := pm.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("got %d playbooks, want 2", len(results))
	}
}

func TestManagerDelete(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	pb := samplePlaybook("Delete Me")
	if err := pm.Create(ctx, pb); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := pm.Delete(ctx, pb.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := pm.Get(ctx, pb.ID)
	if err == nil {
		t.Error("expected error after Delete, got nil")
	}
}

func TestManagerSearch(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	pb := samplePlaybook("Kubernetes Rollout")
	pb.Description = "Procedure for performing kubernetes rollout deployments safely"
	pb.Tags = []string{"kubernetes", "rollout", "deployment"}
	if err := pm.Create(ctx, pb); err != nil {
		t.Fatalf("setup: %v", err)
	}

	results, err := pm.Search(ctx, SearchQuery{
		Text:  "kubernetes rollout",
		Mode:  SearchModeBM25,
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least 1 search result, got 0")
	}
}

func TestManagerRecordExecution(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	pb := samplePlaybook("Execution Test")
	if err := pm.Create(ctx, pb); err != nil {
		t.Fatalf("setup: %v", err)
	}

	rec := &ExecutionRecord{
		PlaybookID:  pb.ID,
		PlaybookVer: 1,
		AgentID:     "agent-1",
		Outcome:     OutcomeSuccess,
		StartedAt:   time.Now(),
		CompletedAt: time.Now().Add(time.Minute),
	}

	if err := pm.RecordExecution(ctx, rec); err != nil {
		t.Fatalf("RecordExecution: %v", err)
	}

	if rec.ID == "" {
		t.Error("expected execution ID to be set")
	}

	updated, err := pm.Get(ctx, pb.ID)
	if err != nil {
		t.Fatalf("Get after RecordExecution: %v", err)
	}
	if updated.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", updated.SuccessCount)
	}
}

func TestManagerListExecutions(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	pb := samplePlaybook("Exec List")
	if err := pm.Create(ctx, pb); err != nil {
		t.Fatalf("setup: %v", err)
	}

	for i := 0; i < 3; i++ {
		rec := &ExecutionRecord{
			PlaybookID:  pb.ID,
			PlaybookVer: 1,
			Outcome:     OutcomeSuccess,
			StartedAt:   time.Now().Add(time.Duration(i) * time.Second),
			CompletedAt: time.Now().Add(time.Duration(i)*time.Second + time.Minute),
		}
		if err := pm.RecordExecution(ctx, rec); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	execs, err := pm.ListExecutions(ctx, pb.ID, 0)
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	if len(execs) != 3 {
		t.Errorf("got %d executions, want 3", len(execs))
	}
}

func TestManagerApplyReflection(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	pb := samplePlaybook("Reflection Target")
	if err := pm.Create(ctx, pb); err != nil {
		t.Fatalf("setup: %v", err)
	}

	originalVersion := pb.Version

	ref := &Reflection{
		WhatWorked:   []string{"step 1 worked"},
		WhatFailed:   []string{"step 2 timed out"},
		Improvements: []string{"add retry logic", "increase timeout"},
		ShouldUpdate: true,
	}

	if err := pm.ApplyReflection(ctx, pb.ID, ref); err != nil {
		t.Fatalf("ApplyReflection: %v", err)
	}

	updated, err := pm.Get(ctx, pb.ID)
	if err != nil {
		t.Fatalf("Get after ApplyReflection: %v", err)
	}

	if updated.Version <= originalVersion {
		t.Errorf("Version = %d, want > %d after reflection", updated.Version, originalVersion)
	}
	if len(updated.Lessons) != 2 {
		t.Errorf("Lessons count = %d, want 2", len(updated.Lessons))
	}
}

func TestManagerStats(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	a := samplePlaybook("Stats Alpha")
	a.Category = "infra"
	if err := pm.Create(ctx, a); err != nil {
		t.Fatalf("setup: %v", err)
	}

	b := samplePlaybook("Stats Beta")
	b.Category = "infra"
	if err := pm.Create(ctx, b); err != nil {
		t.Fatalf("setup: %v", err)
	}

	stats, err := pm.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalPlaybooks != 2 {
		t.Errorf("TotalPlaybooks = %d, want 2", stats.TotalPlaybooks)
	}
	if stats.TotalArchived != 0 {
		t.Errorf("TotalArchived = %d, want 0", stats.TotalArchived)
	}
	if stats.ByCategory["infra"] != 2 {
		t.Errorf("ByCategory[infra] = %d, want 2", stats.ByCategory["infra"])
	}
}

func TestManagerPrune(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	// Create a stale playbook with low confidence and old timestamps.
	stale := samplePlaybook("Old Stale")
	if err := pm.Create(ctx, stale); err != nil {
		t.Fatalf("setup: %v", err)
	}
	stalePB, err := pm.Get(ctx, stale.ID)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	oldTime := time.Now().Add(-180 * 24 * time.Hour) // 180 days ago
	stalePB.CreatedAt = oldTime
	stalePB.UpdatedAt = oldTime
	stalePB.Confidence = 0.1
	if err := pm.store.SavePlaybook(ctx, stalePB); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Create a healthy playbook that should survive.
	healthy := samplePlaybook("Healthy Active")
	if err := pm.Create(ctx, healthy); err != nil {
		t.Fatalf("setup: %v", err)
	}
	healthyPB, err := pm.Get(ctx, healthy.ID)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	healthyPB.LastUsedAt = time.Now()
	healthyPB.Confidence = 0.8
	healthyPB.SuccessCount = 10
	healthyPB.UpdateStats()
	if err := pm.store.SavePlaybook(ctx, healthyPB); err != nil {
		t.Fatalf("setup: %v", err)
	}

	result, err := pm.Prune(ctx, PruneOptions{
		MaxAge:        90 * 24 * time.Hour,
		MinConfidence: 0.3,
		DryRun:        false,
	})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if len(result.Archived) != 1 {
		t.Errorf("Archived count = %d, want 1", len(result.Archived))
	}
	if result.Archived[0] != stale.ID {
		t.Errorf("Archived[0] = %q, want %q", result.Archived[0], stale.ID)
	}

	// Stale playbook should now be archived.
	prunedPB, err := pm.Get(ctx, stale.ID)
	if err != nil {
		t.Fatalf("Get pruned playbook: %v", err)
	}
	if !prunedPB.Archived {
		t.Error("pruned playbook should be archived")
	}
}

func TestManagerPruneDryRun(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	stale := samplePlaybook("Dry Run Stale")
	if err := pm.Create(ctx, stale); err != nil {
		t.Fatalf("setup: %v", err)
	}
	stalePB, err := pm.Get(ctx, stale.ID)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	oldTime := time.Now().Add(-180 * 24 * time.Hour)
	stalePB.CreatedAt = oldTime
	stalePB.UpdatedAt = oldTime
	stalePB.Confidence = 0.1
	if err := pm.store.SavePlaybook(ctx, stalePB); err != nil {
		t.Fatalf("setup: %v", err)
	}

	result, err := pm.Prune(ctx, PruneOptions{
		MaxAge:        90 * 24 * time.Hour,
		MinConfidence: 0.3,
		DryRun:        true,
	})
	if err != nil {
		t.Fatalf("Prune DryRun: %v", err)
	}

	if len(result.Archived) != 1 {
		t.Errorf("Archived count = %d, want 1 (dry run should still report)", len(result.Archived))
	}

	// The playbook should NOT have been changed since it was a dry run.
	pb, err := pm.Get(ctx, stale.ID)
	if err != nil {
		t.Fatalf("Get after dry run: %v", err)
	}
	if pb.Archived {
		t.Error("playbook should not be archived after dry run")
	}
}

// TestManagerIntegrationWorkflow is a full end-to-end integration test that mirrors
// the lifecycle: Create -> Search -> RecordExecution -> ApplyReflection -> Search again.
func TestManagerIntegrationWorkflow(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	// 1. Create a playbook.
	pb := &Playbook{
		Name:        "Incident Response",
		Description: "Handle production incidents and outages",
		Tags:        []string{"incident", "ops", "production"},
		Category:    "operations",
		Steps: []Step{
			{Order: 1, Action: "Alert on-call engineer"},
			{Order: 2, Action: "Assess incident severity"},
			{Order: 3, Action: "Apply mitigation"},
			{Order: 4, Action: "Post-mortem"},
		},
	}
	if err := pm.Create(ctx, pb); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// 2. Search should find it.
	results, err := pm.Search(ctx, SearchQuery{
		Text:  "incident",
		Mode:  SearchModeBM25,
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected search to find the created playbook")
	}

	foundID := results[0].Playbook.ID
	if foundID != pb.ID {
		t.Errorf("found playbook ID = %q, want %q", foundID, pb.ID)
	}

	// 3. Record 3 successful executions.
	for i := 0; i < 3; i++ {
		rec := &ExecutionRecord{
			PlaybookID:  pb.ID,
			PlaybookVer: 1,
			AgentID:     "test-agent",
			Outcome:     OutcomeSuccess,
			StartedAt:   time.Now(),
			CompletedAt: time.Now().Add(5 * time.Minute),
			TaskContext: "production incident drill",
		}
		if err := pm.RecordExecution(ctx, rec); err != nil {
			t.Fatalf("RecordExecution %d: %v", i+1, err)
		}
	}

	afterExec, err := pm.Get(ctx, pb.ID)
	if err != nil {
		t.Fatalf("Get after executions: %v", err)
	}
	if afterExec.SuccessCount != 3 {
		t.Errorf("SuccessCount = %d, want 3", afterExec.SuccessCount)
	}

	// 4. Apply a reflection.
	ref := &Reflection{
		WhatWorked:   []string{"alerting was fast", "mitigation worked"},
		WhatFailed:   []string{"post-mortem took too long"},
		Improvements: []string{"automate post-mortem template"},
		ShouldUpdate: true,
	}
	if err := pm.ApplyReflection(ctx, pb.ID, ref); err != nil {
		t.Fatalf("ApplyReflection: %v", err)
	}

	afterReflect, err := pm.Get(ctx, pb.ID)
	if err != nil {
		t.Fatalf("Get after reflection: %v", err)
	}
	if len(afterReflect.Lessons) == 0 {
		t.Error("expected lessons to be added after reflection")
	}
	if afterReflect.Version <= 1 {
		t.Errorf("Version = %d, want > 1 after update", afterReflect.Version)
	}

	// 5. Search again — playbook should still be findable.
	results2, err := pm.Search(ctx, SearchQuery{
		Text:  "incident response",
		Mode:  SearchModeBM25,
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search after reflection: %v", err)
	}
	if len(results2) == 0 {
		t.Error("expected search to find playbook after reflection and re-index")
	}

	// 6. Stats should reflect one playbook.
	stats, err := pm.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalPlaybooks != 1 {
		t.Errorf("TotalPlaybooks = %d, want 1", stats.TotalPlaybooks)
	}
	if stats.TotalExecs != 3 {
		t.Errorf("TotalExecs = %d, want 3", stats.TotalExecs)
	}
}

func TestNewPlaybookManagerRequiresDataDir(t *testing.T) {
	_, err := NewPlaybookManager(ManagerConfig{})
	if err == nil {
		t.Error("expected error when DataDir is empty")
	}
}

func TestManagerSearchCompositeScore(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	// Create two playbooks with the same keyword so BM25 finds both.
	pbA := samplePlaybook("Deployment Alpha")
	pbA.Description = "deployment procedure for production servers"
	pbA.Tags = []string{"deployment"}
	if err := pm.Create(ctx, pbA); err != nil {
		t.Fatalf("setup: %v", err)
	}

	pbB := samplePlaybook("Deployment Beta")
	pbB.Description = "deployment procedure for staging servers"
	pbB.Tags = []string{"deployment"}
	if err := pm.Create(ctx, pbB); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Give playbook A low confidence (1 success, 9 failures).
	gotA, err := pm.Get(ctx, pbA.ID)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	gotA.SuccessCount = 1
	gotA.FailureCount = 9
	gotA.UpdateStats()
	if err := pm.store.SavePlaybook(ctx, gotA); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Give playbook B high confidence (9 successes, 1 failure).
	gotB, err := pm.Get(ctx, pbB.ID)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	gotB.SuccessCount = 9
	gotB.FailureCount = 1
	gotB.UpdateStats()
	if err := pm.store.SavePlaybook(ctx, gotB); err != nil {
		t.Fatalf("setup: %v", err)
	}

	results, err := pm.Search(ctx, SearchQuery{
		Text:             "deployment",
		Mode:             SearchModeBM25,
		ConfidenceWeight: 0.9,
		Limit:            10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	// High-confidence playbook B should be first.
	if results[0].Playbook.ID != pbB.ID {
		t.Errorf("expected high-confidence playbook %q first, got %q", pbB.ID, results[0].Playbook.ID)
	}
}

func TestManagerSearchCompositeScoreZeroWeightUnchanged(t *testing.T) {
	pm := newTestManager(t)
	ctx := context.Background()

	pbA := samplePlaybook("Scaling Alpha")
	pbA.Description = "scaling procedure for web services"
	pbA.Tags = []string{"scaling"}
	if err := pm.Create(ctx, pbA); err != nil {
		t.Fatalf("setup: %v", err)
	}

	pbB := samplePlaybook("Scaling Beta")
	pbB.Description = "scaling procedure for databases"
	pbB.Tags = []string{"scaling"}
	if err := pm.Create(ctx, pbB); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Set different confidences.
	gotA, err := pm.Get(ctx, pbA.ID)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	gotA.SuccessCount = 1
	gotA.FailureCount = 9
	gotA.UpdateStats()
	if err := pm.store.SavePlaybook(ctx, gotA); err != nil {
		t.Fatalf("setup: %v", err)
	}

	gotB, err := pm.Get(ctx, pbB.ID)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	gotB.SuccessCount = 9
	gotB.FailureCount = 1
	gotB.UpdateStats()
	if err := pm.store.SavePlaybook(ctx, gotB); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Search with zero weight (disabled) — should just return results without reordering.
	results, err := pm.Search(ctx, SearchQuery{
		Text:             "scaling",
		Mode:             SearchModeBM25,
		ConfidenceWeight: 0,
		Limit:            10,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
}

func TestNormalizeScore(t *testing.T) {
	tests := []struct {
		score, min, max float64
		want            float64
	}{
		{score: 10, min: 0, max: 10, want: 1.0},
		{score: 0, min: 0, max: 10, want: 0.0},
		{score: 5, min: 0, max: 10, want: 0.5},
		{score: 5, min: 5, max: 5, want: 1.0}, // all equal
	}
	for _, tc := range tests {
		got := normalizeScore(tc.score, tc.min, tc.max)
		if got != tc.want {
			t.Errorf("normalizeScore(%v, %v, %v) = %v, want %v", tc.score, tc.min, tc.max, got, tc.want)
		}
	}
}
