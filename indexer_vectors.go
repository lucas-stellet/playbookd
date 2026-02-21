//go:build vectors

package playbookd

import (
	blevequery "github.com/blevesearch/bleve/v2/search/query"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
)

// addVectorMapping adds a vector field to the index mapping when built with -tags vectors.
func addVectorMapping(indexMapping *mapping.IndexMappingImpl, dims int) {
	if dims > 0 {
		vectorField := mapping.NewVectorFieldMapping()
		vectorField.Dims = dims
		vectorField.Similarity = "cosine"
		indexMapping.DefaultMapping.AddFieldMappingsAt("embedding", vectorField)
	}
}

func (bi *BleveIndexer) buildVectorRequest(query SearchQuery, limit int) *bleve.SearchRequest {
	if bi.dims == 0 || len(query.Embedding) == 0 {
		return bi.buildBM25Request(query, limit)
	}
	req := bleve.NewSearchRequest(bleve.NewMatchNoneQuery())
	req.AddKNN("embedding", query.Embedding, int64(limit), 1.0)
	req.Size = limit
	return req
}

func (bi *BleveIndexer) buildHybridRequest(query SearchQuery, limit int) *bleve.SearchRequest {
	fields := []string{"name", "description", "tags", "steps", "lessons"}
	fieldQueries := make([]blevequery.Query, 0, len(fields))
	for _, field := range fields {
		q := bleve.NewMatchQuery(query.Text)
		q.SetField(field)
		fieldQueries = append(fieldQueries, q)
	}
	disjQ := bleve.NewDisjunctionQuery(fieldQueries...)
	req := bleve.NewSearchRequest(disjQ)

	if bi.dims > 0 && len(query.Embedding) > 0 {
		req.AddKNN("embedding", query.Embedding, int64(limit), 1.0)
	}

	req.Size = limit
	return req
}
