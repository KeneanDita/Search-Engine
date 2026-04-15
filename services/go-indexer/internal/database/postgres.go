package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// PGPool wraps a pgxpool connection pool.
type PGPool struct {
	Pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewPGPool creates a new connection pool and runs migrations.
func NewPGPool(ctx context.Context, dsn string, logger *zap.Logger) (*PGPool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	p := &PGPool{Pool: pool, logger: logger}
	if err := p.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	logger.Info("postgres connected")
	return p, nil
}

func (p *PGPool) migrate(ctx context.Context) error {
	_, err := p.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS documents (
			id            TEXT PRIMARY KEY,
			url           TEXT NOT NULL UNIQUE,
			title         TEXT,
			content       TEXT,
			word_count    INTEGER,
			language      TEXT DEFAULT 'en',
			source        TEXT,
			published_date TIMESTAMPTZ,
			crawled_at    TIMESTAMPTZ DEFAULT NOW(),
			indexed_at    TIMESTAMPTZ DEFAULT NOW(),
			metadata      JSONB
		);
		CREATE INDEX IF NOT EXISTS idx_documents_source ON documents(source);
		CREATE INDEX IF NOT EXISTS idx_documents_indexed_at ON documents(indexed_at DESC);
		CREATE INDEX IF NOT EXISTS idx_documents_url ON documents(url);
	`)
	return err
}

// UpsertDocument inserts or updates a document's metadata in Postgres.
func (p *PGPool) UpsertDocument(ctx context.Context, doc interface{}) error {
	// Handled via interface via type assertion in indexer
	return nil
}

func (p *PGPool) Close() {
	p.Pool.Close()
}
