//go:build !vectors

package playbookd

import (
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
)

// addVectorMapping is a no-op when built without -tags vectors.
func addVectorMapping(_ *mapping.IndexMappingImpl, _ int) {}

func (bi *BleveIndexer) buildVectorRequest(query SearchQuery, limit int) *bleve.SearchRequest {
	// Without FAISS, fall back to BM25
	return bi.buildBM25Request(query, limit)
}

func (bi *BleveIndexer) buildHybridRequest(query SearchQuery, limit int) *bleve.SearchRequest {
	// Without FAISS, hybrid degrades to BM25 only
	return bi.buildBM25Request(query, limit)
}
