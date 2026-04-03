package tests

import (
	"testing"

	"github.com/searchengine/go-indexer/internal/models"
)

func TestProcessedDocumentJSON(t *testing.T) {
	doc := models.ProcessedDocument{
		ID:        "abc123",
		URL:       "https://example.com",
		Title:     "Test Document",
		Content:   "This is test content",
		Tokens:    []string{"test", "content"},
		Embedding: []float32{0.1, 0.2, 0.3},
		WordCount: 2,
		Language:  "en",
		Source:    "github",
	}

	if doc.ID == "" {
		t.Error("ID should not be empty")
	}
	if len(doc.Embedding) != 3 {
		t.Errorf("expected 3 embedding dimensions, got %d", len(doc.Embedding))
	}
	if doc.Language != "en" {
		t.Errorf("expected language 'en', got '%s'", doc.Language)
	}
}

func TestIndexRequestValidation(t *testing.T) {
	req := models.IndexRequest{
		Documents: []models.ProcessedDocument{
			{ID: "1", URL: "https://a.com", Title: "A"},
			{ID: "2", URL: "https://b.com", Title: "B"},
		},
	}
	if len(req.Documents) != 2 {
		t.Errorf("expected 2 documents, got %d", len(req.Documents))
	}
}
