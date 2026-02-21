package playbookd

// SearchMode determines the search strategy.
type SearchMode string

const (
	SearchModeHybrid  SearchMode = "hybrid"
	SearchModeBM25    SearchMode = "bm25"
	SearchModeVector  SearchMode = "vector"
)

// SearchQuery configures a playbook search.
type SearchQuery struct {
	Text      string     // Natural language query
	Mode      SearchMode // hybrid, bm25, or vector
	Category  string     // Filter by category
	Status    *Status    // Filter by status
	MinScore  float64    // Minimum result score
	Limit     int        // Max results (default 5)
	Embedding []float32  // Pre-computed query embedding (optional)
}

// SearchResult represents a single search hit.
type SearchResult struct {
	Playbook *Playbook
	Score    float64
}

// DefaultSearchLimit is the default number of results returned.
const DefaultSearchLimit = 5

// DefaultMinScore is the default minimum score for results.
const DefaultMinScore = 0.1
