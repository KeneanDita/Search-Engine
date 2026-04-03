package ranking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EmbeddingClient calls the Python NLP service to generate embeddings.
type EmbeddingClient struct {
	nlpURL string
	client *http.Client
}

// NewEmbeddingClient creates a client for the NLP embedding endpoint.
func NewEmbeddingClient(nlpURL string) *EmbeddingClient {
	return &EmbeddingClient{
		nlpURL: nlpURL,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Embed calls the NLP service to embed a query string.
func (e *EmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	payload, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.nlpURL+"/embed", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse embed response: %w", err)
	}
	return result.Embedding, nil
}

// CosineSimilarity computes dot product of two pre-normalised vectors.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}
