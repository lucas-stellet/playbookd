package playbookd

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lucas-stellet/playbookd/embed"

	"github.com/google/uuid"
)

// ManagerConfig configures the PlaybookManager.
type ManagerConfig struct {
	DataDir       string              // Root directory for all data
	EmbedFunc     embed.EmbeddingFunc // Embedding function (nil = BM25 only)
	EmbedDims     int                 // Embedding dimensions (0 = BM25 only)
	AutoReflect   bool                // Automatically trigger reflection after recording
	MaxAge        time.Duration       // Max age before a playbook is prunable (default 90 days)
	MinConfidence float64             // Min confidence for pruning (default 0.3)
	Logger        *slog.Logger        // Logger (nil = slog.Default())
}

// PlaybookManager is the main entry point for the playbookd library.
type PlaybookManager struct {
	store   Store
	indexer Indexer
	embedFn embed.EmbeddingFunc
	cfg     ManagerConfig
	log     *slog.Logger
}

// PruneOptions configures the prune operation.
type PruneOptions struct {
	MaxAge        time.Duration
	MinConfidence float64
	DryRun        bool
}

// PruneResult reports what was pruned.
type PruneResult struct {
	Archived []string // IDs of archived playbooks
}

// Stats holds aggregate statistics.
type Stats struct {
	TotalPlaybooks int
	TotalArchived  int
	ByCategory     map[string]int
	TotalExecs     int
	AvgConfidence  float64
}

// NewPlaybookManager initializes a PlaybookManager with store, indexer, and embedding.
func NewPlaybookManager(cfg ManagerConfig) (*PlaybookManager, error) {
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("data_dir is required")
	}

	// Initialize store
	store, err := NewFileStore(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("create store: %w", err)
	}

	// Initialize embedding function
	embedFn := cfg.EmbedFunc
	if embedFn == nil {
		embedFn = embed.Noop()
	}

	// Initialize indexer
	indexPath := filepath.Join(cfg.DataDir, "index")
	indexer, err := NewBleveIndexer(IndexerConfig{
		Path: indexPath,
		Dims: cfg.EmbedDims,
	})
	if err != nil {
		return nil, fmt.Errorf("create indexer: %w", err)
	}

	// Set defaults
	if cfg.MaxAge == 0 {
		cfg.MaxAge = 90 * 24 * time.Hour
	}
	if cfg.MinConfidence == 0 {
		cfg.MinConfidence = 0.3
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &PlaybookManager{
		store:   store,
		indexer: indexer,
		embedFn: embedFn,
		cfg:     cfg,
		log:     cfg.Logger,
	}, nil
}

// Close shuts down the manager and its resources.
func (pm *PlaybookManager) Close() error {
	return pm.indexer.Close()
}

// Create creates a new playbook, generates its embedding, and indexes it.
func (pm *PlaybookManager) Create(ctx context.Context, pb *Playbook) error {
	if pb.ID == "" {
		pb.ID = uuid.New().String()
	}
	if pb.Slug == "" {
		pb.Slug = slugify(pb.Name)
	}
	if pb.Version == 0 {
		pb.Version = 1
	}

	now := time.Now()
	pb.CreatedAt = now
	pb.UpdatedAt = now
	pb.UpdateStats()

	// Generate embedding
	if err := pm.generateEmbedding(ctx, pb); err != nil {
		return fmt.Errorf("generate embedding: %w", err)
	}

	// Save to store
	if err := pm.store.SavePlaybook(ctx, pb); err != nil {
		return fmt.Errorf("save playbook: %w", err)
	}

	// Index for search
	if err := pm.indexer.Index(ctx, pb); err != nil {
		return fmt.Errorf("index playbook: %w", err)
	}

	return nil
}

// Get retrieves a playbook by ID.
func (pm *PlaybookManager) Get(ctx context.Context, id string) (*Playbook, error) {
	return pm.store.GetPlaybook(ctx, id)
}

// List returns playbooks matching the filter.
func (pm *PlaybookManager) List(ctx context.Context, filter ListFilter) ([]*Playbook, error) {
	return pm.store.ListPlaybooks(ctx, filter)
}

// Update modifies a playbook, re-generates embedding, re-indexes, and increments version.
func (pm *PlaybookManager) Update(ctx context.Context, pb *Playbook) error {
	pb.Version++
	pb.UpdatedAt = time.Now()
	pb.UpdateStats()

	// Re-generate embedding
	if err := pm.generateEmbedding(ctx, pb); err != nil {
		return fmt.Errorf("generate embedding: %w", err)
	}

	// Save to store
	if err := pm.store.SavePlaybook(ctx, pb); err != nil {
		return fmt.Errorf("save playbook: %w", err)
	}

	// Re-index
	if err := pm.indexer.Index(ctx, pb); err != nil {
		return fmt.Errorf("re-index playbook: %w", err)
	}

	return nil
}

// Delete removes a playbook from store and index.
func (pm *PlaybookManager) Delete(ctx context.Context, id string) error {
	if err := pm.store.DeletePlaybook(ctx, id); err != nil {
		return fmt.Errorf("delete playbook: %w", err)
	}
	if err := pm.indexer.Remove(ctx, id); err != nil {
		return fmt.Errorf("remove from index: %w", err)
	}
	return nil
}

// Search performs hybrid BM25 + vector search and hydrates results with full playbook data.
func (pm *PlaybookManager) Search(ctx context.Context, query SearchQuery) ([]SearchResult, error) {
	// Generate query embedding if not provided and we have an embed function
	if len(query.Embedding) == 0 && query.Text != "" {
		emb, err := pm.embedFn(ctx, query.Text)
		if err != nil {
			// Non-fatal: fall back to BM25 only
			pm.log.Warn("embedding failed, falling back to BM25", "error", err)
			query.Mode = SearchModeBM25
		} else {
			query.Embedding = emb
		}
	}

	results, err := pm.indexer.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	// Hydrate results with full playbook data
	hydrated := make([]SearchResult, 0, len(results))
	for _, r := range results {
		pb, err := pm.store.GetPlaybook(ctx, r.Playbook.ID)
		if err != nil {
			continue // Skip if playbook was deleted between search and fetch
		}
		hydrated = append(hydrated, SearchResult{
			Playbook: pb,
			Score:    r.Score,
		})
	}

	// Composite score blending
	if query.ConfidenceWeight > 0 && len(hydrated) > 0 {
		w := query.ConfidenceWeight
		if w > 1 {
			w = 1
		}

		// Find min/max text scores for normalization
		minScore, maxScore := hydrated[0].Score, hydrated[0].Score
		for _, r := range hydrated[1:] {
			if r.Score < minScore {
				minScore = r.Score
			}
			if r.Score > maxScore {
				maxScore = r.Score
			}
		}

		// Blend and re-sort
		for i := range hydrated {
			norm := normalizeScore(hydrated[i].Score, minScore, maxScore)
			hydrated[i].Score = (1-w)*norm + w*hydrated[i].Playbook.Confidence
		}

		sort.Slice(hydrated, func(i, j int) bool {
			return hydrated[i].Score > hydrated[j].Score
		})
	}

	return hydrated, nil
}

// normalizeScore applies min-max normalization to [0,1].
// Returns 1.0 if all scores are equal.
func normalizeScore(score, min, max float64) float64 {
	if max == min {
		return 1.0
	}
	return (score - min) / (max - min)
}

// RecordExecution saves an execution record and updates the playbook stats.
func (pm *PlaybookManager) RecordExecution(ctx context.Context, rec *ExecutionRecord) error {
	if rec.ID == "" {
		rec.ID = uuid.New().String()
	}

	// Save execution
	if err := pm.store.SaveExecution(ctx, rec); err != nil {
		return fmt.Errorf("save execution: %w", err)
	}

	// Update playbook stats
	pb, err := pm.store.GetPlaybook(ctx, rec.PlaybookID)
	if err != nil {
		return fmt.Errorf("get playbook for stats update: %w", err)
	}

	switch rec.Outcome {
	case OutcomeSuccess:
		pb.SuccessCount++
	case OutcomeFailure:
		pb.FailureCount++
	case OutcomePartial:
		// Partial counts as a full success for stats
		pb.SuccessCount++
	}

	pb.LastUsedAt = rec.CompletedAt
	pb.UpdateStats()

	if err := pm.store.SavePlaybook(ctx, pb); err != nil {
		return fmt.Errorf("save updated playbook: %w", err)
	}

	// Re-index with updated stats
	if err := pm.indexer.Index(ctx, pb); err != nil {
		return fmt.Errorf("re-index playbook: %w", err)
	}

	// Auto-reflect if enabled
	if pm.cfg.AutoReflect && rec.Reflection != nil && rec.Reflection.ShouldUpdate {
		if err := pm.ApplyReflection(ctx, rec.PlaybookID, rec.Reflection); err != nil {
			// Non-fatal: log but don't fail the recording
			pm.log.Warn("auto-reflect failed", "playbook_id", rec.PlaybookID, "error", err)
		}
	}

	return nil
}

// ListExecutions returns recent executions for a playbook.
func (pm *PlaybookManager) ListExecutions(ctx context.Context, playbookID string, limit int) ([]*ExecutionRecord, error) {
	return pm.store.ListExecutions(ctx, playbookID, limit)
}

// ApplyReflection applies improvements from a reflection to a playbook.
func (pm *PlaybookManager) ApplyReflection(ctx context.Context, playbookID string, ref *Reflection) error {
	pb, err := pm.store.GetPlaybook(ctx, playbookID)
	if err != nil {
		return fmt.Errorf("get playbook: %w", err)
	}

	// Add lessons from improvements
	for _, improvement := range ref.Improvements {
		lesson := Lesson{
			ID:          uuid.New().String(),
			Content:     improvement,
			LearnedFrom: "reflection",
			LearnedAt:   time.Now(),
			Applies:     "general",
			Confidence:  0.5,
		}
		pb.Lessons = append(pb.Lessons, lesson)
	}

	// Update the playbook (increments version, re-embeds, re-indexes)
	return pm.Update(ctx, pb)
}

// Prune archives playbooks that are stale or have low confidence.
func (pm *PlaybookManager) Prune(ctx context.Context, opts PruneOptions) (*PruneResult, error) {
	if opts.MaxAge == 0 {
		opts.MaxAge = pm.cfg.MaxAge
	}
	if opts.MinConfidence == 0 {
		opts.MinConfidence = pm.cfg.MinConfidence
	}

	playbooks, err := pm.store.ListPlaybooks(ctx, ListFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}

	result := &PruneResult{}
	cutoff := time.Now().Add(-opts.MaxAge)

	for _, pb := range playbooks {
		if pb.Archived {
			continue
		}

		shouldPrune := false

		// Low confidence + old age
		if pb.Confidence < opts.MinConfidence && pb.UpdatedAt.Before(cutoff) {
			shouldPrune = true
		}

		// Stale (unused too long)
		if !pb.LastUsedAt.IsZero() && pb.LastUsedAt.Before(cutoff) {
			shouldPrune = true
		}

		// Never used + old + low confidence
		if pb.LastUsedAt.IsZero() && pb.CreatedAt.Before(cutoff) && pb.Confidence < opts.MinConfidence {
			shouldPrune = true
		}

		if shouldPrune {
			result.Archived = append(result.Archived, pb.ID)
			if !opts.DryRun {
				pb.Archived = true
				pb.UpdatedAt = time.Now()
				if err := pm.store.SavePlaybook(ctx, pb); err != nil {
					return nil, fmt.Errorf("archive playbook %s: %w", pb.ID, err)
				}
				if err := pm.indexer.Remove(ctx, pb.ID); err != nil {
					return nil, fmt.Errorf("remove archived playbook from index %s: %w", pb.ID, err)
				}
			}
		}
	}

	return result, nil
}

// Reindex rebuilds the entire search index from stored playbooks.
func (pm *PlaybookManager) Reindex(ctx context.Context) error {
	playbooks, err := pm.store.ListPlaybooks(ctx, ListFilter{})
	if err != nil {
		return err
	}
	return pm.indexer.Reindex(ctx, playbooks)
}

// Stats returns aggregate statistics across all playbooks.
func (pm *PlaybookManager) Stats(ctx context.Context) (*Stats, error) {
	playbooks, err := pm.store.ListPlaybooks(ctx, ListFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}

	stats := &Stats{
		TotalPlaybooks: len(playbooks),
		ByCategory:     make(map[string]int),
	}

	var totalConfidence float64
	for _, pb := range playbooks {
		if pb.Archived {
			stats.TotalArchived++
		}
		if pb.Category != "" {
			stats.ByCategory[pb.Category]++
		}
		totalConfidence += pb.Confidence
		stats.TotalExecs += pb.SuccessCount + pb.FailureCount
	}

	if len(playbooks) > 0 {
		stats.AvgConfidence = totalConfidence / float64(len(playbooks))
	}

	return stats, nil
}

// generateEmbedding creates an embedding for the playbook's text content.
func (pm *PlaybookManager) generateEmbedding(ctx context.Context, pb *Playbook) error {
	var stepActions []string
	for _, s := range pb.Steps {
		stepActions = append(stepActions, s.Action)
	}

	text := embed.TextForPlaybook(pb.Name, pb.Description, pb.Tags, stepActions)
	emb, err := pm.embedFn(ctx, text)
	if err != nil {
		return err
	}
	pb.Embedding = emb
	return nil
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a name to a URL-safe slug.
func slugify(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = nonAlphanumeric.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}
