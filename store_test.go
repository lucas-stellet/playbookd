package playbookd

import (
	"context"
	"testing"
	"time"
)

func newTestPlaybook(id, name string) *Playbook {
	now := time.Now()
	return &Playbook{
		ID:          id,
		Name:        name,
		Slug:        slugify(name),
		Description: "Test playbook " + name,
		Tags:        []string{"test", "unit"},
		Category:    "testing",
		Status:      StatusDraft,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestNewFileStore(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	if fs == nil {
		t.Fatal("expected non-nil FileStore")
	}
}

func TestFileStoreSaveAndGetPlaybook(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	ctx := context.Background()
	pb := newTestPlaybook("pb-001", "My Playbook")

	if err := fs.SavePlaybook(ctx, pb); err != nil {
		t.Fatalf("SavePlaybook: %v", err)
	}

	got, err := fs.GetPlaybook(ctx, "pb-001")
	if err != nil {
		t.Fatalf("GetPlaybook: %v", err)
	}

	if got.ID != pb.ID {
		t.Errorf("ID = %q, want %q", got.ID, pb.ID)
	}
	if got.Name != pb.Name {
		t.Errorf("Name = %q, want %q", got.Name, pb.Name)
	}
	if got.Status != pb.Status {
		t.Errorf("Status = %q, want %q", got.Status, pb.Status)
	}
}

func TestFileStoreGetPlaybookNotFound(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileStore(dir)
	ctx := context.Background()

	_, err := fs.GetPlaybook(ctx, "does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing playbook, got nil")
	}
}

func TestFileStoreSavePlaybookOverwrites(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileStore(dir)
	ctx := context.Background()

	pb := newTestPlaybook("pb-001", "Original")
	if err := fs.SavePlaybook(ctx, pb); err != nil {
		t.Fatalf("setup: %v", err)
	}

	pb.Name = "Updated"
	if err := fs.SavePlaybook(ctx, pb); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := fs.GetPlaybook(ctx, "pb-001")
	if err != nil {
		t.Fatalf("GetPlaybook: %v", err)
	}
	if got.Name != "Updated" {
		t.Errorf("Name = %q, want %q", got.Name, "Updated")
	}
}

func TestFileStoreListPlaybooks(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileStore(dir)
	ctx := context.Background()

	active := StatusActive
	pbs := []*Playbook{
		{ID: "a", Name: "Alpha", Status: StatusActive, Category: "ops", Tags: []string{"tag1"}, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "b", Name: "Beta", Status: StatusDraft, Category: "ops", Tags: []string{"tag1", "tag2"}, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "c", Name: "Gamma", Status: StatusActive, Category: "dev", Tags: []string{"tag2"}, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	for _, pb := range pbs {
		if err := fs.SavePlaybook(ctx, pb); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	t.Run("no filter returns all", func(t *testing.T) {
		results, err := fs.ListPlaybooks(ctx, ListFilter{})
		if err != nil {
			t.Fatalf("ListPlaybooks: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("got %d playbooks, want 3", len(results))
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		results, err := fs.ListPlaybooks(ctx, ListFilter{Status: &active})
		if err != nil {
			t.Fatalf("ListPlaybooks: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d playbooks, want 2", len(results))
		}
	})

	t.Run("filter by category", func(t *testing.T) {
		results, err := fs.ListPlaybooks(ctx, ListFilter{Category: "ops"})
		if err != nil {
			t.Fatalf("ListPlaybooks: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d playbooks, want 2", len(results))
		}
	})

	t.Run("filter by tags", func(t *testing.T) {
		results, err := fs.ListPlaybooks(ctx, ListFilter{Tags: []string{"tag1", "tag2"}})
		if err != nil {
			t.Fatalf("ListPlaybooks: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("got %d playbooks, want 1", len(results))
		}
		if results[0].ID != "b" {
			t.Errorf("got playbook %q, want %q", results[0].ID, "b")
		}
	})

	t.Run("limit results", func(t *testing.T) {
		results, err := fs.ListPlaybooks(ctx, ListFilter{Limit: 2})
		if err != nil {
			t.Fatalf("ListPlaybooks: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d playbooks, want 2", len(results))
		}
	})
}

func TestFileStoreDeletePlaybook(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileStore(dir)
	ctx := context.Background()

	pb := newTestPlaybook("pb-del", "To Delete")
	if err := fs.SavePlaybook(ctx, pb); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := fs.DeletePlaybook(ctx, "pb-del"); err != nil {
		t.Fatalf("DeletePlaybook: %v", err)
	}

	_, err := fs.GetPlaybook(ctx, "pb-del")
	if err == nil {
		t.Error("expected error after deleting, got nil")
	}
}

func TestFileStoreDeletePlaybookNotExist(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileStore(dir)
	ctx := context.Background()

	// Deleting a non-existent playbook should not error.
	if err := fs.DeletePlaybook(ctx, "ghost"); err != nil {
		t.Errorf("expected no error for non-existent delete, got: %v", err)
	}
}

func TestFileStoreSaveAndListExecutions(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileStore(dir)
	ctx := context.Background()

	pb := newTestPlaybook("pb-exec", "Exec Playbook")
	if err := fs.SavePlaybook(ctx, pb); err != nil {
		t.Fatalf("setup: %v", err)
	}

	base := time.Now()
	recs := []*ExecutionRecord{
		{
			ID:          "exec-1",
			PlaybookID:  "pb-exec",
			PlaybookVer: 1,
			Outcome:     OutcomeSuccess,
			StartedAt:   base.Add(-2 * time.Hour),
			CompletedAt: base.Add(-2*time.Hour + 5*time.Minute),
		},
		{
			ID:          "exec-2",
			PlaybookID:  "pb-exec",
			PlaybookVer: 1,
			Outcome:     OutcomeFailure,
			StartedAt:   base.Add(-1 * time.Hour),
			CompletedAt: base.Add(-1*time.Hour + 3*time.Minute),
		},
		{
			ID:          "exec-3",
			PlaybookID:  "pb-exec",
			PlaybookVer: 1,
			Outcome:     OutcomeSuccess,
			StartedAt:   base,
			CompletedAt: base.Add(4 * time.Minute),
		},
	}
	for _, rec := range recs {
		if err := fs.SaveExecution(ctx, rec); err != nil {
			t.Fatalf("SaveExecution %s: %v", rec.ID, err)
		}
	}

	t.Run("list all", func(t *testing.T) {
		results, err := fs.ListExecutions(ctx, "pb-exec", 0)
		if err != nil {
			t.Fatalf("ListExecutions: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("got %d records, want 3", len(results))
		}
		// Should be newest first.
		if results[0].ID != "exec-3" {
			t.Errorf("first result = %q, want exec-3 (newest)", results[0].ID)
		}
	})

	t.Run("with limit", func(t *testing.T) {
		results, err := fs.ListExecutions(ctx, "pb-exec", 2)
		if err != nil {
			t.Fatalf("ListExecutions: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d records, want 2", len(results))
		}
	})

	t.Run("no executions for unknown playbook", func(t *testing.T) {
		results, err := fs.ListExecutions(ctx, "unknown-pb", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("got %d records, want 0", len(results))
		}
	})
}

func TestFileStoreDeleteAlsoRemovesExecutions(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileStore(dir)
	ctx := context.Background()

	pb := newTestPlaybook("pb-cleanup", "Cleanup")
	if err := fs.SavePlaybook(ctx, pb); err != nil {
		t.Fatalf("setup: %v", err)
	}

	rec := &ExecutionRecord{
		ID:         "exec-cleanup",
		PlaybookID: "pb-cleanup",
		Outcome:    OutcomeSuccess,
		StartedAt:  time.Now(),
	}
	if err := fs.SaveExecution(ctx, rec); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := fs.DeletePlaybook(ctx, "pb-cleanup"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Executions should also be gone.
	results, err := fs.ListExecutions(ctx, "pb-cleanup", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 executions after delete, got %d", len(results))
	}
}
