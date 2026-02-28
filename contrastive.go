package playbookd

import "context"

// Default thresholds for contrastive search.
const (
	DefaultPositiveMinConfidence = 0.5
	DefaultNegativeMaxConfidence = 0.3
)

// ContrastiveQuery extends SearchQuery with confidence thresholds for splitting
// results into positive (proven) and negative (failed) groups.
type ContrastiveQuery struct {
	SearchQuery
	PositiveMinConfidence float64 // Minimum confidence for positive group (default 0.5)
	NegativeMaxConfidence float64 // Maximum confidence for negative group (default 0.3)
	IncludeNeutral        bool    // Whether to include neutral results
}

// ContrastiveResults holds search results split by confidence into positive,
// negative, and optionally neutral groups.
type ContrastiveResults struct {
	Positive []SearchResult
	Negative []SearchResult
	Neutral  []SearchResult // Only populated if IncludeNeutral was true
	Query    string
}

// SearchWithContext performs a contrastive search, splitting results into
// positive (high confidence), negative (low confidence), and optionally
// neutral groups based on the playbook's Wilson confidence score.
func (pm *PlaybookManager) SearchWithContext(ctx context.Context, cq ContrastiveQuery) (*ContrastiveResults, error) {
	// Apply defaults
	if cq.PositiveMinConfidence == 0 {
		cq.PositiveMinConfidence = DefaultPositiveMinConfidence
	}
	if cq.NegativeMaxConfidence == 0 {
		cq.NegativeMaxConfidence = DefaultNegativeMaxConfidence
	}

	// Save original limit and search with expanded limit to capture more candidates
	originalLimit := cq.Limit
	if originalLimit == 0 {
		originalLimit = DefaultSearchLimit
	}
	cq.SearchQuery.Limit = originalLimit * 3
	cq.SearchQuery.MinScore = 0 // Capture low-quality matches too

	results, err := pm.Search(ctx, cq.SearchQuery)
	if err != nil {
		return nil, err
	}

	cr := &ContrastiveResults{
		Query: cq.Text,
	}

	// Split by Wilson confidence (real confidence, not blended score)
	for _, r := range results {
		switch {
		case r.Playbook.Confidence >= cq.PositiveMinConfidence:
			cr.Positive = append(cr.Positive, r)
		case r.Playbook.Confidence <= cq.NegativeMaxConfidence:
			cr.Negative = append(cr.Negative, r)
		default:
			if cq.IncludeNeutral {
				cr.Neutral = append(cr.Neutral, r)
			}
		}
	}

	// Cap each group to original limit
	if len(cr.Positive) > originalLimit {
		cr.Positive = cr.Positive[:originalLimit]
	}
	if len(cr.Negative) > originalLimit {
		cr.Negative = cr.Negative[:originalLimit]
	}
	if len(cr.Neutral) > originalLimit {
		cr.Neutral = cr.Neutral[:originalLimit]
	}

	return cr, nil
}
