package models

// ProcessedDocument is the NLP-enriched document consumed from the Redis queue.
type ProcessedDocument struct {
	ID            string                 `json:"id"`
	URL           string                 `json:"url"`
	Title         string                 `json:"title"`
	Content       string                 `json:"content"`
	Tokens        []string               `json:"tokens"`
	Keyphrases    []string               `json:"keyphrases"`
	Entities      map[string][]string    `json:"entities"`
	Embedding     []float32              `json:"embedding"`
	WordCount     int                    `json:"word_count"`
	Language      string                 `json:"language"`
	Source        string                 `json:"source"`
	PublishedDate *string                `json:"published_date,omitempty"`
	CrawledAt     float64                `json:"crawled_at,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// IndexRequest is the HTTP payload for manual indexing.
type IndexRequest struct {
	Documents []ProcessedDocument `json:"documents"`
}

// IndexResponse is the result of an index operation.
type IndexResponse struct {
	Indexed int      `json:"indexed"`
	Failed  int      `json:"failed"`
	IDs     []string `json:"ids"`
	Errors  []string `json:"errors,omitempty"`
}
