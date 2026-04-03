-- Search Engine — PostgreSQL initialisation
-- Run automatically on first container start

CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Documents table (metadata mirror of OpenSearch)
CREATE TABLE IF NOT EXISTS documents (
    id             TEXT PRIMARY KEY,
    url            TEXT NOT NULL,
    title          TEXT,
    content        TEXT,
    word_count     INTEGER DEFAULT 0,
    language       TEXT DEFAULT 'en',
    source         TEXT,
    published_date TIMESTAMPTZ,
    crawled_at     TIMESTAMPTZ DEFAULT NOW(),
    indexed_at     TIMESTAMPTZ DEFAULT NOW(),
    metadata       JSONB DEFAULT '{}'::jsonb,
    CONSTRAINT documents_url_unique UNIQUE (url)
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_documents_source      ON documents (source);
CREATE INDEX IF NOT EXISTS idx_documents_indexed_at  ON documents (indexed_at DESC);
CREATE INDEX IF NOT EXISTS idx_documents_language    ON documents (language);
CREATE INDEX IF NOT EXISTS idx_documents_word_count  ON documents (word_count);

-- GIN index for full-text search on title+content (fallback)
CREATE INDEX IF NOT EXISTS idx_documents_fts ON documents
    USING GIN (to_tsvector('english', coalesce(title,'') || ' ' || coalesce(content,'')));

-- GIN index for trigram similarity on URL
CREATE INDEX IF NOT EXISTS idx_documents_url_trgm ON documents USING GIN (url gin_trgm_ops);

-- Search queries log (analytics)
CREATE TABLE IF NOT EXISTS search_logs (
    id          BIGSERIAL PRIMARY KEY,
    query       TEXT NOT NULL,
    mode        TEXT DEFAULT 'hybrid',
    result_count INTEGER DEFAULT 0,
    duration_ms  INTEGER DEFAULT 0,
    ip_hash      TEXT,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_search_logs_created_at ON search_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_search_logs_query ON search_logs USING GIN (query gin_trgm_ops);

-- Crawl queue status table
CREATE TABLE IF NOT EXISTS crawl_jobs (
    id          BIGSERIAL PRIMARY KEY,
    url         TEXT NOT NULL,
    status      TEXT DEFAULT 'pending',  -- pending | running | done | failed
    source      TEXT,
    attempts    INTEGER DEFAULT 0,
    last_error  TEXT,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_crawl_jobs_status ON crawl_jobs (status);
