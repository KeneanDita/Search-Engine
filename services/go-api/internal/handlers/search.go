package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/searchengine/go-api/internal/cache"
	"github.com/searchengine/go-api/internal/models"
	"github.com/searchengine/go-api/internal/ranking"
)

const (
	defaultPageSize = 10
	maxPageSize     = 50
	defaultMode     = "hybrid"
)

// SearchHandler holds search handler dependencies.
type SearchHandler struct {
	osAddr     string
	nlpURL     string
	cache      *cache.RedisCache
	embedder   *ranking.EmbeddingClient
	logger     *zap.Logger
	httpClient *http.Client
}

// NewSearchHandler creates a search handler.
func NewSearchHandler(
	osAddr, nlpURL string,
	cache *cache.RedisCache,
	logger *zap.Logger,
) *SearchHandler {
	return &SearchHandler{
		osAddr:     osAddr,
		nlpURL:     nlpURL,
		cache:      cache,
		embedder:   ranking.NewEmbeddingClient(nlpURL),
		logger:     logger,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Search handles GET /search?q=...
func (h *SearchHandler) Search(c *fiber.Ctx) error {
	start := time.Now()

	req := &models.SearchRequest{
		Page:     1,
		PageSize: defaultPageSize,
		Mode:     defaultMode,
	}
	if err := c.QueryParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid query parameters", Code: 400, Details: err.Error(),
		})
	}
	if req.Query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "q parameter is required", Code: 400,
		})
	}
	if req.PageSize > maxPageSize {
		req.PageSize = maxPageSize
	}
	if req.Page < 1 {
		req.Page = 1
	}

	// Normalise mode
	req.Mode = strings.ToLower(req.Mode)
	switch req.Mode {
	case "keyword", "semantic", "hybrid":
	default:
		req.Mode = defaultMode
	}

	// Check cache
	cacheKey := cache.SearchKey(req.Query, req.Mode, req.Source, req.Language, req.Page, req.PageSize)
	var cached models.SearchResponse
	if h.cache.Get(c.Context(), cacheKey, &cached) {
		h.logger.Debug("cache_hit", zap.String("query", req.Query))
		return c.JSON(cached)
	}

	var (
		keywordHits  []models.SearchHit
		semanticHits []models.SearchHit
		total        int64
	)

	ctx := c.Context()

	switch req.Mode {
	case "keyword":
		hits, tot, err := h.keywordSearch(ctx, req)
		if err != nil {
			h.logger.Error("keyword search failed", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "search failed", Code: 500})
		}
		keywordHits, total = hits, tot

	case "semantic":
		hits, tot, err := h.semanticSearch(ctx, req)
		if err != nil {
			h.logger.Error("semantic search failed", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{Error: "search failed", Code: 500})
		}
		semanticHits, total = hits, tot

	case "hybrid":
		var kErr, sErr error
		var kh, sh []models.SearchHit
		doneCh := make(chan struct{}, 2)

		go func() {
			kh, _, kErr = h.keywordSearch(ctx, req)
			doneCh <- struct{}{}
		}()
		go func() {
			sh, _, sErr = h.semanticSearch(ctx, req)
			doneCh <- struct{}{}
		}()
		<-doneCh
		<-doneCh

		keywordHits = kh
		semanticHits = sh

		if kErr != nil {
			h.logger.Warn("keyword search error in hybrid", zap.Error(kErr))
		}
		if sErr != nil {
			h.logger.Warn("semantic search error in hybrid", zap.Error(sErr))
		}
		// Use max total
		if len(keywordHits) > 0 {
			total = int64(len(keywordHits))
		}
		if int64(len(semanticHits)) > total {
			total = int64(len(semanticHits))
		}
	}

	// Merge / rank
	var hits []models.SearchHit
	switch req.Mode {
	case "keyword":
		hits = keywordHits
	case "semantic":
		hits = semanticHits
	case "hybrid":
		hits = ranking.FuseResults(keywordHits, semanticHits, ranking.DefaultHybridConfig())
		total = int64(len(hits))
	}

	// Apply min score filter
	if req.MinScore > 0 {
		filtered := hits[:0]
		for _, h := range hits {
			if h.Score >= req.MinScore {
				filtered = append(filtered, h)
			}
		}
		hits = filtered
		total = int64(len(hits))
	}

	// Paginate
	from := (req.Page - 1) * req.PageSize
	if from >= len(hits) {
		hits = nil
	} else {
		end := from + req.PageSize
		if end > len(hits) {
			end = len(hits)
		}
		hits = hits[from:end]
	}

	totalPages := int(math.Ceil(float64(total) / float64(req.PageSize)))
	if totalPages < 1 {
		totalPages = 1
	}

	resp := models.SearchResponse{
		Query:      req.Query,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: totalPages,
		Mode:       req.Mode,
		DurationMs: time.Since(start).Milliseconds(),
		Hits:       hits,
	}

	h.cache.Set(c.Context(), cacheKey, resp)
	return c.JSON(resp)
}

// keywordSearch performs BM25-based keyword search via OpenSearch.
func (h *SearchHandler) keywordSearch(ctx context.Context, req *models.SearchRequest) ([]models.SearchHit, int64, error) {
	from := (req.Page - 1) * req.PageSize
	query := h.buildKeywordQuery(req.Query, req.Source, req.Language, req.DateFrom, req.DateTo, from, req.PageSize*3)

	body, err := json.Marshal(query)
	if err != nil {
		return nil, 0, err
	}
	result, err := h.osSearch(ctx, body)
	if err != nil {
		return nil, 0, err
	}
	return h.parseOSHits(result, "keyword"), result.Hits.Total.Value, nil
}

// semanticSearch performs vector similarity search via OpenSearch kNN.
func (h *SearchHandler) semanticSearch(ctx context.Context, req *models.SearchRequest) ([]models.SearchHit, int64, error) {
	embedding, err := h.embedder.Embed(ctx, req.Query)
	if err != nil {
		return nil, 0, fmt.Errorf("embed query: %w", err)
	}

	query := h.buildSemanticQuery(embedding, req.Source, req.Language, req.PageSize*3)
	body, err := json.Marshal(query)
	if err != nil {
		return nil, 0, err
	}
	result, err := h.osSearch(ctx, body)
	if err != nil {
		return nil, 0, err
	}
	return h.parseOSHits(result, "semantic"), result.Hits.Total.Value, nil
}

func (h *SearchHandler) buildKeywordQuery(q, source, lang, dateFrom, dateTo string, from, size int) map[string]interface{} {
	must := []interface{}{
		map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":     q,
				"fields":    []string{"title^3", "content^1", "keyphrases^2"},
				"type":      "best_fields",
				"fuzziness": "AUTO",
			},
		},
	}
	filters := h.buildFilters(source, lang, dateFrom, dateTo)
	query := map[string]interface{}{
		"from": from,
		"size": size,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must":   must,
				"filter": filters,
			},
		},
		"highlight": map[string]interface{}{
			"fields": map[string]interface{}{
				"content": map[string]interface{}{"fragment_size": 200, "number_of_fragments": 2},
				"title":   map[string]interface{}{},
			},
		},
		"_source": []string{"id", "url", "title", "content", "source", "published_date", "keyphrases", "entities", "metadata"},
	}
	return query
}

func (h *SearchHandler) buildSemanticQuery(embedding []float32, source, lang string, size int) map[string]interface{} {
	filters := h.buildFilters(source, lang, "", "")
	query := map[string]interface{}{
		"size": size,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []interface{}{
					map[string]interface{}{
						"knn": map[string]interface{}{
							"embedding": map[string]interface{}{
								"vector": embedding,
								"k":      size,
							},
						},
					},
				},
				"filter": filters,
			},
		},
		"_source": []string{"id", "url", "title", "content", "source", "published_date", "keyphrases", "entities", "metadata"},
	}
	return query
}

func (h *SearchHandler) buildFilters(source, lang, dateFrom, dateTo string) []interface{} {
	var filters []interface{}
	if source != "" {
		filters = append(filters, map[string]interface{}{"term": map[string]interface{}{"source": source}})
	}
	if lang != "" {
		filters = append(filters, map[string]interface{}{"term": map[string]interface{}{"language": lang}})
	}
	if dateFrom != "" || dateTo != "" {
		dateRange := map[string]interface{}{}
		if dateFrom != "" {
			dateRange["gte"] = dateFrom
		}
		if dateTo != "" {
			dateRange["lte"] = dateTo
		}
		filters = append(filters, map[string]interface{}{"range": map[string]interface{}{"published_date": dateRange}})
	}
	return filters
}

// osSearchResult is the minimal OpenSearch response shape we need.
type osSearchResult struct {
	Hits struct {
		Total struct {
			Value int64 `json:"value"`
		} `json:"total"`
		Hits []struct {
			ID        string                 `json:"_id"`
			Score     float64                `json:"_score"`
			Source    map[string]interface{} `json:"_source"`
			Highlight map[string][]string    `json:"highlight"`
		} `json:"hits"`
	} `json:"hits"`
}

func (h *SearchHandler) osSearch(ctx context.Context, body []byte) (*osSearchResult, error) {
	url := fmt.Sprintf("%s/search_documents/_search", h.osAddr)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("opensearch request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result osSearchResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse opensearch response: %w", err)
	}
	return &result, nil
}

func (h *SearchHandler) parseOSHits(result *osSearchResult, mode string) []models.SearchHit {
	hits := make([]models.SearchHit, 0, len(result.Hits.Hits))
	for _, h := range result.Hits.Hits {
		src := h.Source
		hit := models.SearchHit{
			ID:     getString(src, "id"),
			URL:    getString(src, "url"),
			Title:  getString(src, "title"),
			Score:  h.Score,
			Source: getString(src, "source"),
		}
		if hit.ID == "" {
			hit.ID = h.ID
		}

		// Snippet: prefer highlight, fall back to content truncation
		if hl, ok := h.Highlight["content"]; ok && len(hl) > 0 {
			hit.Snippet = strings.Join(hl, " … ")
		} else {
			content := getString(src, "content")
			if len(content) > 300 {
				content = content[:300] + "…"
			}
			hit.Snippet = content
		}

		if pd, ok := src["published_date"].(string); ok {
			hit.PublishedDate = &pd
		}
		if kp, ok := src["keyphrases"].([]interface{}); ok {
			for _, k := range kp {
				if s, ok := k.(string); ok {
					hit.Keyphrases = append(hit.Keyphrases, s)
				}
			}
		}
		hits = append(hits, hit)
	}
	return hits
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
