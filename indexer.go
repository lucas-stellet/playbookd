package playbookd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	blevequery "github.com/blevesearch/bleve/v2/search/query"
)

// Indexer defines the search index interface.
type Indexer interface {
	Index(ctx context.Context, pb *Playbook) error
	Remove(ctx context.Context, id string) error
	Search(ctx context.Context, query SearchQuery) ([]SearchResult, error)
	Reindex(ctx context.Context, playbooks []*Playbook) error
	Close() error
}

// BleveIndexer implements Indexer using Bleve with optional FAISS vector support.
type BleveIndexer struct {
	index     bleve.Index
	indexPath string
	dims      int // embedding dimensions, 0 means no vector support
}

var _ Indexer = (*BleveIndexer)(nil)

// bleveDoc is the document structure indexed by Bleve.
type bleveDoc struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Tags        string    `json:"tags"`
	Category    string    `json:"category"`
	Status      string    `json:"status"`
	Steps       string    `json:"steps"`
	Lessons     string    `json:"lessons"`
	Confidence  float64   `json:"confidence"`
	SuccessRate float64   `json:"success_rate"`
	Embedding   []float32 `json:"embedding,omitempty"`
}

// IndexerConfig configures the Bleve indexer.
type IndexerConfig struct {
	Path string // Directory for the Bleve index
	Dims int    // Embedding dimensions (0 = BM25 only, no vector field)
}

// NewBleveIndexer creates or opens a Bleve index at the given path.
func NewBleveIndexer(cfg IndexerConfig) (*BleveIndexer, error) {
	// Try to open existing index first
	idx, err := bleve.Open(cfg.Path)
	if err == nil {
		return &BleveIndexer{
			index:     idx,
			indexPath: cfg.Path,
			dims:      cfg.Dims,
		}, nil
	}

	// If the path exists but bleve.Open failed, the index is corrupt or incompatible.
	if _, statErr := os.Stat(cfg.Path); statErr == nil {
		return nil, fmt.Errorf("open bleve index: %w", err)
	}

	// Create new index with mapping
	indexMapping := buildBaseIndexMapping()
	addVectorMapping(indexMapping, cfg.Dims)

	idx, err = bleve.New(cfg.Path, indexMapping)
	if err != nil {
		return nil, fmt.Errorf("create bleve index: %w", err)
	}

	return &BleveIndexer{
		index:     idx,
		indexPath: cfg.Path,
		dims:      cfg.Dims,
	}, nil
}

// buildBaseIndexMapping creates the Bleve index mapping with text fields for BM25.
func buildBaseIndexMapping() *mapping.IndexMappingImpl {
	indexMapping := bleve.NewIndexMapping()
	docMapping := bleve.NewDocumentMapping()

	// Text fields for BM25 search
	textField := bleve.NewTextFieldMapping()
	textField.Analyzer = "en"
	textField.Store = false

	docMapping.AddFieldMappingsAt("name", textField)
	docMapping.AddFieldMappingsAt("description", textField)
	docMapping.AddFieldMappingsAt("tags", textField)
	docMapping.AddFieldMappingsAt("steps", textField)
	docMapping.AddFieldMappingsAt("lessons", textField)

	// Keyword fields for filtering
	keywordField := bleve.NewKeywordFieldMapping()
	keywordField.Store = false
	docMapping.AddFieldMappingsAt("category", keywordField)
	docMapping.AddFieldMappingsAt("status", keywordField)

	// Numeric fields
	numericField := bleve.NewNumericFieldMapping()
	numericField.Store = false
	docMapping.AddFieldMappingsAt("confidence", numericField)
	docMapping.AddFieldMappingsAt("success_rate", numericField)

	indexMapping.DefaultMapping = docMapping
	return indexMapping
}

// Index adds or updates a playbook in the search index.
func (bi *BleveIndexer) Index(_ context.Context, pb *Playbook) error {
	doc := playbookToDoc(pb)
	if err := bi.index.Index(pb.ID, doc); err != nil {
		return fmt.Errorf("index playbook %s: %w", pb.ID, err)
	}
	return nil
}

// Remove deletes a playbook from the search index.
func (bi *BleveIndexer) Remove(_ context.Context, id string) error {
	if err := bi.index.Delete(id); err != nil {
		return fmt.Errorf("remove playbook %s: %w", id, err)
	}
	return nil
}

// Search executes a search query against the index.
func (bi *BleveIndexer) Search(_ context.Context, query SearchQuery) ([]SearchResult, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}

	mode := query.Mode
	if mode == "" {
		mode = SearchModeHybrid
	}

	var searchReq *bleve.SearchRequest

	switch mode {
	case SearchModeBM25:
		searchReq = bi.buildBM25Request(query, limit)
	case SearchModeVector:
		searchReq = bi.buildVectorRequest(query, limit)
	case SearchModeHybrid:
		searchReq = bi.buildHybridRequest(query, limit)
	default:
		return nil, fmt.Errorf("unsupported search mode: %s", mode)
	}

	// Apply status filter if specified
	if query.Status != nil {
		filterQuery := bleve.NewTermQuery(string(*query.Status))
		filterQuery.SetField("status")
		conjQuery := bleve.NewConjunctionQuery(searchReq.Query, filterQuery)
		searchReq.Query = conjQuery
	}

	// Apply category filter if specified
	if query.Category != "" {
		filterQuery := bleve.NewTermQuery(query.Category)
		filterQuery.SetField("category")
		conjQuery := bleve.NewConjunctionQuery(searchReq.Query, filterQuery)
		searchReq.Query = conjQuery
	}

	results, err := bi.index.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("bleve search: %w", err)
	}

	searchResults := make([]SearchResult, 0, len(results.Hits))
	for _, hit := range results.Hits {
		if query.MinScore > 0 && hit.Score < query.MinScore {
			continue
		}
		searchResults = append(searchResults, SearchResult{
			Playbook: &Playbook{ID: hit.ID},
			Score:    hit.Score,
		})
	}

	return searchResults, nil
}

// Reindex indexes all provided playbooks in a single batch. It does not remove
// stale entries for playbooks not present in the list; callers that need a full
// rebuild should delete the index directory and create a new BleveIndexer.
func (bi *BleveIndexer) Reindex(_ context.Context, playbooks []*Playbook) error {
	batch := bi.index.NewBatch()
	for _, pb := range playbooks {
		doc := playbookToDoc(pb)
		if err := batch.Index(pb.ID, doc); err != nil {
			return fmt.Errorf("batch index %s: %w", pb.ID, err)
		}
	}
	return bi.index.Batch(batch)
}

// Close closes the Bleve index.
func (bi *BleveIndexer) Close() error {
	return bi.index.Close()
}

func (bi *BleveIndexer) buildBM25Request(query SearchQuery, limit int) *bleve.SearchRequest {
	// Search across each indexed text field individually, then combine with OR.
	// NewMatchQuery against the _all composite field does not work correctly when
	// individual fields use the "en" analyzer, because _all uses a different analyzer.
	fields := []string{"name", "description", "tags", "steps", "lessons"}
	fieldQueries := make([]blevequery.Query, 0, len(fields))
	for _, field := range fields {
		q := bleve.NewMatchQuery(query.Text)
		q.SetField(field)
		fieldQueries = append(fieldQueries, q)
	}
	disjQ := bleve.NewDisjunctionQuery(fieldQueries...)
	req := bleve.NewSearchRequest(disjQ)
	req.Size = limit
	return req
}

// playbookToDoc converts a Playbook to the indexed document format.
func playbookToDoc(pb *Playbook) bleveDoc {
	var stepActions []string
	for _, s := range pb.Steps {
		stepActions = append(stepActions, s.Action)
	}

	var lessonContents []string
	for _, l := range pb.Lessons {
		lessonContents = append(lessonContents, l.Content)
	}

	return bleveDoc{
		Name:        pb.Name,
		Description: pb.Description,
		Tags:        strings.Join(pb.Tags, " "),
		Category:    pb.Category,
		Status:      string(pb.Status),
		Steps:       strings.Join(stepActions, " "),
		Lessons:     strings.Join(lessonContents, " "),
		Confidence:  pb.Confidence,
		SuccessRate: pb.SuccessRate,
		Embedding:   pb.Embedding,
	}
}
