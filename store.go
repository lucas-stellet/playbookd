package playbookd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("not found")

// Compile-time check that FileStore implements Store.
var _ Store = (*FileStore)(nil)

// Store defines the persistence interface for playbooks and executions.
type Store interface {
	SavePlaybook(ctx context.Context, pb *Playbook) error
	GetPlaybook(ctx context.Context, id string) (*Playbook, error)
	ListPlaybooks(ctx context.Context, filter ListFilter) ([]*Playbook, error)
	DeletePlaybook(ctx context.Context, id string) error
	SaveExecution(ctx context.Context, rec *ExecutionRecord) error
	ListExecutions(ctx context.Context, playbookID string, limit int) ([]*ExecutionRecord, error)
}

// FileStore implements Store using JSON files on disk.
type FileStore struct {
	dataDir string
	mu      sync.RWMutex
}

// NewFileStore creates a new file-based store at the given directory.
func NewFileStore(dataDir string) (*FileStore, error) {
	playbooksDir := filepath.Join(dataDir, "playbooks")
	executionsDir := filepath.Join(dataDir, "executions")

	for _, dir := range []string{playbooksDir, executionsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return &FileStore{dataDir: dataDir}, nil
}

func (fs *FileStore) playbookPath(id string) string {
	return filepath.Join(fs.dataDir, "playbooks", id+".json")
}

func (fs *FileStore) executionDir(playbookID string) string {
	return filepath.Join(fs.dataDir, "executions", playbookID)
}

func (fs *FileStore) executionPath(playbookID, execID string) string {
	return filepath.Join(fs.executionDir(playbookID), execID+".json")
}

// SavePlaybook persists a playbook to disk using atomic write (temp file + rename).
func (fs *FileStore) SavePlaybook(_ context.Context, pb *Playbook) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	return atomicWriteJSON(fs.playbookPath(pb.ID), pb)
}

// GetPlaybook loads a playbook by ID.
func (fs *FileStore) GetPlaybook(_ context.Context, id string) (*Playbook, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	path := fs.playbookPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("playbook %s: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("read playbook %s: %w", id, err)
	}

	var pb Playbook
	if err := json.Unmarshal(data, &pb); err != nil {
		return nil, fmt.Errorf("unmarshal playbook %s: %w", id, err)
	}

	return &pb, nil
}

// ListPlaybooks returns all playbooks matching the filter.
func (fs *FileStore) ListPlaybooks(_ context.Context, filter ListFilter) ([]*Playbook, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	dir := filepath.Join(fs.dataDir, "playbooks")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read playbooks dir: %w", err)
	}

	var playbooks []*Playbook
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			// Intentionally skip unreadable files; a single corrupt file
			// should not prevent listing the rest of the playbooks.
			continue
		}

		var pb Playbook
		if err := json.Unmarshal(data, &pb); err != nil {
			// Intentionally skip malformed JSON files for the same reason.
			continue
		}

		if !matchesFilter(&pb, filter) {
			continue
		}

		playbooks = append(playbooks, &pb)
	}

	// Sort by confidence descending
	sort.Slice(playbooks, func(i, j int) bool {
		return playbooks[i].Confidence > playbooks[j].Confidence
	})

	if filter.Limit > 0 && len(playbooks) > filter.Limit {
		playbooks = playbooks[:filter.Limit]
	}

	return playbooks, nil
}

// DeletePlaybook removes a playbook and its executions from disk.
func (fs *FileStore) DeletePlaybook(_ context.Context, id string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	path := fs.playbookPath(id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete playbook %s: %w", id, err)
	}

	// Also remove executions directory
	execDir := fs.executionDir(id)
	if err := os.RemoveAll(execDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete executions for %s: %w", id, err)
	}

	return nil
}

// SaveExecution persists an execution record to disk.
func (fs *FileStore) SaveExecution(_ context.Context, rec *ExecutionRecord) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	dir := fs.executionDir(rec.PlaybookID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create execution dir: %w", err)
	}

	return atomicWriteJSON(fs.executionPath(rec.PlaybookID, rec.ID), rec)
}

// ListExecutions returns recent executions for a playbook, newest first.
func (fs *FileStore) ListExecutions(_ context.Context, playbookID string, limit int) ([]*ExecutionRecord, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	dir := fs.executionDir(playbookID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read executions dir: %w", err)
	}

	var records []*ExecutionRecord
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			// Intentionally skip unreadable files; a single corrupt record
			// should not prevent listing the remaining executions.
			continue
		}

		var rec ExecutionRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			// Intentionally skip malformed JSON files for the same reason.
			continue
		}

		records = append(records, &rec)
	}

	// Sort by started_at descending (newest first)
	sort.Slice(records, func(i, j int) bool {
		return records[i].StartedAt.After(records[j].StartedAt)
	})

	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}

	return records, nil
}

// matchesFilter checks if a playbook matches the given filter criteria.
func matchesFilter(pb *Playbook, filter ListFilter) bool {
	if !filter.IncludeArchived && pb.Archived {
		return false
	}
	if filter.Category != "" && pb.Category != filter.Category {
		return false
	}
	if len(filter.Tags) > 0 {
		tagSet := make(map[string]bool, len(pb.Tags))
		for _, t := range pb.Tags {
			tagSet[t] = true
		}
		for _, required := range filter.Tags {
			if !tagSet[required] {
				return false
			}
		}
	}
	return true
}

// atomicWriteJSON writes data as JSON to a file atomically (temp file + rename).
func atomicWriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
