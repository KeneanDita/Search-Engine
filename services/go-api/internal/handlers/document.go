package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/searchengine/go-api/internal/models"
)

// DocumentHandler serves individual document retrieval.
type DocumentHandler struct {
	osAddr     string
	httpClient *http.Client
	logger     *zap.Logger
}

// NewDocumentHandler creates a document handler.
func NewDocumentHandler(osAddr string, logger *zap.Logger) *DocumentHandler {
	return &DocumentHandler{
		osAddr:     osAddr,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
	}
}

// GetDocument handles GET /document/:id
func (h *DocumentHandler) GetDocument(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "document id required", Code: 400,
		})
	}

	doc, err := h.fetchDoc(c.Context(), id)
	if err != nil {
		h.logger.Error("fetch document failed", zap.String("id", id), zap.Error(err))
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error: "document not found", Code: 404,
		})
	}
	return c.JSON(doc)
}

func (h *DocumentHandler) fetchDoc(ctx context.Context, id string) (*models.DocumentResponse, error) {
	url := fmt.Sprintf("%s/search_documents/_doc/%s", h.osAddr, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("not found")
	}
	data, _ := io.ReadAll(resp.Body)

	var result struct {
		Found  bool                   `json:"found"`
		Source map[string]interface{} `json:"_source"`
	}
	if err := json.Unmarshal(data, &result); err != nil || !result.Found {
		return nil, fmt.Errorf("document not found")
	}
	src := result.Source
	doc := &models.DocumentResponse{
		ID:        getString(src, "id"),
		URL:       getString(src, "url"),
		Title:     getString(src, "title"),
		Content:   getString(src, "content"),
		WordCount: getInt(src, "word_count"),
		Language:  getString(src, "language"),
		Source:    getString(src, "source"),
	}
	if doc.ID == "" {
		doc.ID = id
	}
	if pd, ok := src["published_date"].(string); ok {
		doc.PublishedDate = &pd
	}
	if kp, ok := src["keyphrases"].([]interface{}); ok {
		for _, k := range kp {
			if s, ok := k.(string); ok {
				doc.Keyphrases = append(doc.Keyphrases, s)
			}
		}
	}
	return doc, nil
}

func getInt(m map[string]interface{}, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}
