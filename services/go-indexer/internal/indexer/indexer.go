package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/searchengine/go-indexer/internal/models"
)

// Indexer orchestrates writing to OpenSearch and PostgreSQL.
type Indexer struct {
	os     *OSClient
	pg     *pgxpool.Pool
	logger *zap.Logger
}

// New creates an Indexer with the given dependencies.
func New(os *OSClient, pg *pgxpool.Pool, logger *zap.Logger) *Indexer {
	return &Indexer{os: os, pg: pg, logger: logger}
}

// IndexDocuments indexes a batch of processed documents.
func (idx *Indexer) IndexDocuments(ctx context.Context, docs []models.ProcessedDocument) (int, []error) {
	if len(docs) == 0 {
		return 0, nil
	}

	// Prepare OpenSearch bulk payload
	osDocs := make([]map[string]interface{}, 0, len(docs))
	for _, d := range docs {
		osDocs = append(osDocs, docToMap(d))
	}

	indexed, err := idx.os.BulkIndex(ctx, osDocs)
	if err != nil {
		idx.logger.Error("bulk index failed", zap.Error(err))
		return 0, []error{err}
	}

	// Write metadata to Postgres in batch
	errs := idx.upsertPG(ctx, docs)

	idx.logger.Info("indexed batch",
		zap.Int("opensearch_indexed", indexed),
		zap.Int("total", len(docs)),
		zap.Int("pg_errors", len(errs)),
	)
	return indexed, errs
}

func (idx *Indexer) upsertPG(ctx context.Context, docs []models.ProcessedDocument) []error {
	var errs []error
	for _, d := range docs {
		metaJSON, _ := json.Marshal(d.Metadata)
		var publishedDate *time.Time
		if d.PublishedDate != nil && *d.PublishedDate != "" {
			if t, err := time.Parse(time.RFC3339, *d.PublishedDate); err == nil {
				publishedDate = &t
			}
		}
		_, err := idx.pg.Exec(ctx, `
			INSERT INTO documents (id, url, title, content, word_count, language, source, published_date, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
				title         = EXCLUDED.title,
				content       = EXCLUDED.content,
				word_count    = EXCLUDED.word_count,
				source        = EXCLUDED.source,
				published_date = EXCLUDED.published_date,
				indexed_at    = NOW(),
				metadata      = EXCLUDED.metadata
		`, d.ID, d.URL, d.Title, d.Content, d.WordCount, d.Language, d.Source, publishedDate, metaJSON)
		if err != nil {
			idx.logger.Error("pg upsert failed", zap.String("id", d.ID), zap.Error(err))
			errs = append(errs, fmt.Errorf("pg upsert %s: %w", d.ID, err))
		}
	}
	return errs
}

func docToMap(d models.ProcessedDocument) map[string]interface{} {
	m := map[string]interface{}{
		"id":          d.ID,
		"url":         d.URL,
		"title":       d.Title,
		"content":     d.Content,
		"tokens":      d.Tokens,
		"keyphrases":  d.Keyphrases,
		"entities":    d.Entities,
		"word_count":  d.WordCount,
		"language":    d.Language,
		"source":      d.Source,
		"crawled_at":  d.CrawledAt,
		"metadata":    d.Metadata,
	}
	if len(d.Embedding) > 0 {
		m["embedding"] = d.Embedding
	}
	if d.PublishedDate != nil {
		m["published_date"] = *d.PublishedDate
	}
	return m
}
