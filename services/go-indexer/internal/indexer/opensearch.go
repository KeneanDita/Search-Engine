package indexer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"

	opensearch "github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"go.uber.org/zap"
)

const indexName = "search_documents"

// OSClient wraps the OpenSearch client.
type OSClient struct {
	client *opensearch.Client
	logger *zap.Logger
}

// NewOSClient creates a configured OpenSearch client.
func NewOSClient(addr string, logger *zap.Logger) (*OSClient, error) {
	cfg := opensearch.Config{
		Addresses: []string{addr},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	client, err := opensearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("new opensearch client: %w", err)
	}
	c := &OSClient{client: client, logger: logger}
	if err := c.ensureIndex(context.Background()); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *OSClient) ensureIndex(ctx context.Context) error {
	mapping := map[string]interface{}{
		"settings": map[string]interface{}{
			"number_of_shards":   1,
			"number_of_replicas": 0,
			"analysis": map[string]interface{}{
				"analyzer": map[string]interface{}{
					"custom_analyzer": map[string]interface{}{
						"type":      "custom",
						"tokenizer": "standard",
						"filter":    []string{"lowercase", "stop", "snowball"},
					},
				},
			},
		},
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"id":             {"type": "keyword"},
				"url":            {"type": "keyword"},
				"title":          {"type": "text", "analyzer": "custom_analyzer"},
				"content":        {"type": "text", "analyzer": "custom_analyzer"},
				"tokens":         {"type": "keyword"},
				"keyphrases":     {"type": "keyword"},
				"entities":       {"type": "object", "enabled": false},
				"embedding":      {"type": "knn_vector", "dimension": 384},
				"word_count":     {"type": "integer"},
				"language":       {"type": "keyword"},
				"source":         {"type": "keyword"},
				"published_date": {"type": "date", "ignore_malformed": true},
				"crawled_at":     {"type": "date", "format": "epoch_second"},
				"metadata":       {"type": "object", "enabled": false},
			},
		},
	}

	body, _ := json.Marshal(mapping)
	req := opensearchapi.IndicesCreateRequest{
		Index: indexName,
		Body:  bytes.NewReader(body),
	}
	resp, err := req.Do(ctx, c.client)
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	defer resp.Body.Close()
	// 400 means index already exists — that's fine
	if resp.IsError() && resp.StatusCode != 400 {
		c.logger.Warn("index creation response", zap.String("status", resp.Status()))
	} else {
		c.logger.Info("opensearch index ready", zap.String("index", indexName))
	}
	return nil
}

// IndexDocument indexes a single document.
func (c *OSClient) IndexDocument(ctx context.Context, doc map[string]interface{}) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal doc: %w", err)
	}
	docID, _ := doc["id"].(string)
	req := opensearchapi.IndexRequest{
		Index:      indexName,
		DocumentID: docID,
		Body:       bytes.NewReader(body),
		Refresh:    "false",
	}
	resp, err := req.Do(ctx, c.client)
	if err != nil {
		return fmt.Errorf("index doc: %w", err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("opensearch error: %s", resp.Status())
	}
	return nil
}

// BulkIndex indexes multiple documents efficiently.
func (c *OSClient) BulkIndex(ctx context.Context, docs []map[string]interface{}) (int, error) {
	if len(docs) == 0 {
		return 0, nil
	}
	var buf bytes.Buffer
	for _, doc := range docs {
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": indexName,
				"_id":    doc["id"],
			},
		}
		metaBytes, _ := json.Marshal(meta)
		docBytes, _ := json.Marshal(doc)
		buf.Write(metaBytes)
		buf.WriteByte('\n')
		buf.Write(docBytes)
		buf.WriteByte('\n')
	}

	req := opensearchapi.BulkRequest{
		Body:    &buf,
		Refresh: "false",
	}
	resp, err := req.Do(ctx, c.client)
	if err != nil {
		return 0, fmt.Errorf("bulk index: %w", err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return 0, fmt.Errorf("bulk index error: %s", resp.Status())
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	errCount := 0
	if hasErrors, ok := result["errors"].(bool); ok && hasErrors {
		items, _ := result["items"].([]interface{})
		for _, item := range items {
			m, _ := item.(map[string]interface{})
			if idx, ok := m["index"].(map[string]interface{}); ok {
				if idx["error"] != nil {
					errCount++
					c.logger.Warn("bulk index item error", zap.Any("error", idx["error"]))
				}
			}
		}
	}
	return len(docs) - errCount, nil
}
