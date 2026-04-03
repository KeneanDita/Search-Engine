package models

// SearchRequest is the parsed query from the API caller.
type SearchRequest struct {
	Query     string  `json:"q" query:"q"`
	Page      int     `json:"page" query:"page"`
	PageSize  int     `json:"page_size" query:"page_size"`
	Mode      string  `json:"mode" query:"mode"`       // "keyword" | "semantic" | "hybrid"
	Source    string  `json:"source" query:"source"`   // filter by source
	Language  string  `json:"language" query:"language"`
	DateFrom  string  `json:"date_from" query:"date_from"`
	DateTo    string  `json:"date_to" query:"date_to"`
	MinScore  float64 `json:"min_score" query:"min_score"`
}

// SearchHit is a single result document.
type SearchHit struct {
	ID            string                 `json:"id"`
	URL           string                 `json:"url"`
	Title         string                 `json:"title"`
	Snippet       string                 `json:"snippet"`
	Score         float64                `json:"score"`
	KeywordScore  float64                `json:"keyword_score,omitempty"`
	SemanticScore float64                `json:"semantic_score,omitempty"`
	Source        string                 `json:"source"`
	PublishedDate *string                `json:"published_date,omitempty"`
	Keyphrases    []string               `json:"keyphrases,omitempty"`
	Entities      map[string][]string    `json:"entities,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// SearchResponse wraps results + metadata.
type SearchResponse struct {
	Query      string      `json:"query"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
	Mode       string      `json:"mode"`
	DurationMs int64       `json:"duration_ms"`
	Hits       []SearchHit `json:"hits"`
}

// DocumentResponse is a full single-document response.
type DocumentResponse struct {
	ID            string                 `json:"id"`
	URL           string                 `json:"url"`
	Title         string                 `json:"title"`
	Content       string                 `json:"content"`
	Tokens        []string               `json:"tokens,omitempty"`
	Keyphrases    []string               `json:"keyphrases,omitempty"`
	Entities      map[string][]string    `json:"entities,omitempty"`
	WordCount     int                    `json:"word_count"`
	Language      string                 `json:"language"`
	Source        string                 `json:"source"`
	PublishedDate *string                `json:"published_date,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Details string `json:"details,omitempty"`
}
