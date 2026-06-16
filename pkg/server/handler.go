package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/solo/fulltext-search/pkg/index"
	"github.com/solo/fulltext-search/pkg/query"
)

type Handler struct {
	indexManager *index.IndexManager
}

func NewHandler(im *index.IndexManager) *Handler {
	return &Handler{indexManager: im}
}

func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

func (h *Handler) ReadyCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
	})
}

type CreateIndexRequest struct {
	Mappings *index.IndexMapping  `json:"mappings"`
	Settings *index.IndexSettings `json:"settings"`
}

func (h *Handler) CreateIndex(c *gin.Context) {
	name := c.Param("name")

	var req CreateIndexRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var mapping *index.IndexMapping
	var settings *index.IndexSettings
	if req.Mappings != nil {
		mapping = req.Mappings
	}
	if req.Settings != nil {
		settings = req.Settings
	}

	if err := h.indexManager.CreateIndex(name, mapping, settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"acknowledged": true,
		"index":        name,
	})
}

func (h *Handler) DeleteIndex(c *gin.Context) {
	name := c.Param("name")

	confirm := c.GetHeader("X-Confirm-Delete")
	if confirm != "true" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "dangerous operation: require X-Confirm-Delete: true header",
		})
		return
	}

	if err := h.indexManager.DeleteIndex(name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"acknowledged": true,
	})
}

func (h *Handler) GetIndex(c *gin.Context) {
	name := c.Param("name")

	idx, ok := h.indexManager.GetIndex(name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "index not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		name: gin.H{
			"mappings": idx.Meta().Mapping,
			"settings": idx.Meta().Settings,
			"stats": gin.H{
				"docs": gin.H{
					"count": idx.DocCount(),
				},
			},
		},
	})
}

func (h *Handler) ListIndexes(c *gin.Context) {
	indexes := h.indexManager.ListIndexes()
	result := make([]gin.H, 0, len(indexes))
	for _, meta := range indexes {
		result = append(result, gin.H{
			"name":    meta.Name,
			"health":  "green",
			"status":  "open",
			"docs":    meta.DocCount,
			"created": meta.Created,
		})
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) RefreshIndex(c *gin.Context) {
	name := c.Param("name")

	idx, ok := h.indexManager.GetIndex(name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "index not found"})
		return
	}

	if err := idx.Refresh(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"_shards": gin.H{
			"total":      1,
			"successful": 1,
			"failed":     0,
		},
	})
}

func (h *Handler) ForceMerge(c *gin.Context) {
	name := c.Param("name")

	maxSegments := 1
	if v := c.Query("max_num_segments"); v != "" {
		if n, err := parseInt(v); err == nil {
			maxSegments = n
		}
	}

	idx, ok := h.indexManager.GetIndex(name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "index not found"})
		return
	}

	if err := idx.ForceMerge(maxSegments); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"_shards": gin.H{
			"total":      1,
			"successful": 1,
			"failed":     0,
		},
	})
}

func (h *Handler) GetDocument(c *gin.Context) {
	indexName := c.Param("name")
	docID := c.Param("id")

	idx, ok := h.indexManager.GetIndex(indexName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "index not found"})
		return
	}

	doc, ok := idx.GetDocument(docID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"_index":   indexName,
			"_id":      docID,
			"found":    false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"_index":   indexName,
		"_id":      doc.ID,
		"_version": doc.Version,
		"found":    true,
		"_source":  doc.Fields,
	})
}

func (h *Handler) PutDocument(c *gin.Context) {
	indexName := c.Param("name")
	docID := c.Param("id")

	var source map[string]interface{}
	if err := c.ShouldBindJSON(&source); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	idx, ok := h.indexManager.GetIndex(indexName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "index not found"})
		return
	}

	doc := &index.Document{
		ID:     docID,
		Fields: source,
	}

	isNew := true
	if existing, ok := idx.GetDocument(docID); ok {
		doc.Version = existing.Version
		isNew = false
	}

	if err := idx.AddDocument(doc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	status := http.StatusCreated
	if !isNew {
		status = http.StatusOK
	}

	c.JSON(status, gin.H{
		"_index":   indexName,
		"_id":      docID,
		"_version": doc.Version,
		"result":   map[bool]string{true: "created", false: "updated"}[isNew],
	})
}

func (h *Handler) DeleteDocument(c *gin.Context) {
	indexName := c.Param("name")
	docID := c.Param("id")

	idx, ok := h.indexManager.GetIndex(indexName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "index not found"})
		return
	}

	if err := idx.DeleteDocument(docID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"_index":   indexName,
		"_id":      docID,
		"result":   "deleted",
	})
}

type BulkAction struct {
	Index  *map[string]interface{} `json:"index,omitempty"`
	Delete *map[string]interface{} `json:"delete,omitempty"`
	Create *map[string]interface{} `json:"create,omitempty"`
	Update *map[string]interface{} `json:"update,omitempty"`
}

func (h *Handler) Bulk(c *gin.Context) {
	indexName := c.Param("name")

	idx, ok := h.indexManager.GetIndex(indexName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "index not found"})
		return
	}

	var items []map[string]interface{}
	if err := c.ShouldBindJSON(&items); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var results []gin.H
	i := 0
	for i < len(items) {
		action := items[i]
		i++

		if indexAction, ok := action["index"]; ok {
			meta := indexAction.(map[string]interface{})
			docID := ""
			if id, ok := meta["_id"]; ok {
				docID = id.(string)
			}

			var source map[string]interface{}
			if i < len(items) {
				source = items[i]
				i++
			}

			doc := &index.Document{
				ID:     docID,
				Fields: source,
			}
			if err := idx.AddDocument(doc); err != nil {
				results = append(results, gin.H{
					"index": gin.H{
						"_id":    docID,
						"error":  err.Error(),
						"status": 500,
					},
				})
			} else {
				results = append(results, gin.H{
					"index": gin.H{
						"_index":   indexName,
						"_id":      docID,
						"_version": doc.Version,
						"status":   201,
					},
				})
			}
		} else if deleteAction, ok := action["delete"]; ok {
			meta := deleteAction.(map[string]interface{})
			docID := ""
			if id, ok := meta["_id"]; ok {
				docID = id.(string)
			}

			if err := idx.DeleteDocument(docID); err != nil {
				results = append(results, gin.H{
					"delete": gin.H{
						"_id":    docID,
						"error":  err.Error(),
						"status": 500,
					},
				})
			} else {
				results = append(results, gin.H{
					"delete": gin.H{
						"_index":   indexName,
						"_id":      docID,
						"status":   200,
						"result":   "deleted",
					},
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"took":   0,
		"errors": false,
		"items":  results,
	})
}

func (h *Handler) Search(c *gin.Context) {
	indexName := c.Param("name")

	idx, ok := h.indexManager.GetIndex(indexName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "index not found"})
		return
	}

	body, err := c.GetRawData()
	if err != nil || len(body) == 0 {
		searcher := query.NewSearcher(idx)
		req := &query.SearchRequest{
			Query: &query.MatchAllQuery{},
			Size:  10,
		}
		result, err := searcher.Search(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		h.sendSearchResult(c, indexName, result)
		return
	}

	req, err := query.ParseSearchRequest(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	searcher := query.NewSearcher(idx)
	result, err := searcher.Search(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.sendSearchResult(c, indexName, result)
}

func (h *Handler) sendSearchResult(c *gin.Context, indexName string, result *query.SearchResult) {
	hits := make([]gin.H, 0, len(result.Hits))
	for _, hit := range result.Hits {
		h := gin.H{
			"_index":  indexName,
			"_id":     hit.ID,
			"_score":  hit.Score,
			"_source": hit.Source,
		}
		if hit.Highlight != nil && len(hit.Highlight) > 0 {
			h["highlight"] = hit.Highlight
		}
		if len(hit.SortValues) > 0 {
			h["sort"] = hit.SortValues
		}
		hits = append(hits, h)
	}

	response := gin.H{
		"took": 0,
		"timed_out": false,
		"_shards": gin.H{
			"total":      1,
			"successful": 1,
			"skipped":    0,
			"failed":     0,
		},
		"hits": gin.H{
			"total": gin.H{
				"value":    result.Total,
				"relation": "eq",
			},
			"max_score": result.MaxScore,
			"hits":      hits,
		},
	}

	if result.Aggregations != nil && len(result.Aggregations) > 0 {
		aggs := make(map[string]interface{})
		for name, agg := range result.Aggregations {
			buckets := make([]gin.H, 0, len(agg.Buckets))
			for _, b := range agg.Buckets {
				buckets = append(buckets, gin.H{
					"key":       b.Key,
					"key_as_string": b.KeyAsString,
					"doc_count": b.DocCount,
				})
			}
			aggs[name] = gin.H{
				"buckets": buckets,
			}
		}
		response["aggregations"] = aggs
	}

	c.JSON(http.StatusOK, response)
}

func (h *Handler) Suggest(c *gin.Context) {
	indexName := c.Param("name")

	idx, ok := h.indexManager.GetIndex(indexName)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "index not found"})
		return
	}

	field := c.Query("field")
	if field == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field parameter is required"})
		return
	}

	prefix := c.Query("prefix")
	size := 10
	if v := c.Query("size"); v != "" {
		if n, err := parseInt(v); err == nil {
			size = n
		}
	}

	fuzzy := c.Query("fuzzy") == "true"
	fuzziness := 2
	if v := c.Query("fuzziness"); v != "" {
		if n, err := parseInt(v); err == nil {
			fuzziness = n
		}
	}

	searcher := query.NewSearcher(idx)
	req := &query.SuggestRequest{
		Field:     field,
		Prefix:    prefix,
		Size:      size,
		Fuzzy:     fuzzy,
		Fuzziness: fuzziness,
	}
	results := searcher.Suggest(req)

	suggestions := make([]gin.H, 0, len(results))
	for _, r := range results {
		suggestions = append(suggestions, gin.H{
			"text":  r.Text,
			"score": r.Score,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"suggestions": suggestions,
	})
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
