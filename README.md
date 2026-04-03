# Hybrid Search Engine

A production-ready hybrid search engine built with **Go** and **Python**, using only free and open-source tools. Combines BM25 keyword ranking with semantic vector search via Reciprocal Rank Fusion (RRF).

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Search Engine Stack                          │
│                                                                     │
│  ┌─────────────────────────────────┐  ┌──────────────────────────┐ │
│  │       Python Layer              │  │       Go Layer           │ │
│  │                                 │  │                          │ │
│  │  ┌──────────────────────────┐   │  │  ┌────────────────────┐  │ │
│  │  │   python-crawler :8000   │   │  │  │  go-api :8080      │  │ │
│  │  │  • Tavily, Exa, Serper   │   │  │  │  • Fiber REST API  │  │ │
│  │  │  • NewsAPI, GitHub, SO   │   │  │  │  • Rate limiting   │  │ │
│  │  │  • Unsplash, BS4         │   │  │  │  • Hybrid ranking  │  │ │
│  │  │  • Playwright (JS pages) │   │  │  │  • Redis cache     │  │ │
│  │  └────────────┬─────────────┘   │  │  └────────┬───────────┘  │ │
│  │               │ Redis Queue     │  │           │ OpenSearch   │ │
│  │  ┌────────────▼─────────────┐   │  │  ┌────────▼───────────┐  │ │
│  │  │   python-nlp :8001       │   │  │  │  go-indexer :8081  │  │ │
│  │  │  • spaCy NER             │   │  │  │  • Bulk indexing   │  │ │
│  │  │  • NLTK tokenisation     │───┼──┼─▶│  • Postgres meta  │  │ │
│  │  │  • all-MiniLM embeddings │   │  │  │  • Queue consumer │  │ │
│  │  │  • HTML cleaning         │   │  │  └────────────────────┘  │ │
│  │  └──────────────────────────┘   │  └──────────────────────────┘ │
│  └─────────────────────────────────┘                               │
│                                                                     │
│  ┌──────────────────┐  ┌───────────────────┐  ┌─────────────────┐ │
│  │  OpenSearch :9200│  │  PostgreSQL :5432  │  │  Redis :6379    │ │
│  │  • kNN vectors   │  │  • Doc metadata    │  │  • Work queues  │ │
│  │  • BM25 search   │  │  • Search logs     │  │  • Result cache │ │
│  │  • Inverted idx  │  │  • Crawl jobs      │  │  • Visited URLs │ │
│  └──────────────────┘  └───────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
API Keys / Seed Query
        │
        ▼
 python-crawler ──── fetches from Tavily/Exa/Serper/NewsAPI/GitHub/SO/Unsplash
        │                        scrapes URLs with BS4 or Playwright
        │ Redis queue: "queue:crawled"
        ▼
 python-nlp ──────── cleans HTML → tokenises → NER → embeds (384-dim)
        │
        │ Redis queue: "queue:processed"
        ▼
 go-indexer ──────── bulk writes to OpenSearch + Postgres metadata
        │
        ▼
 go-api ──────────── GET /api/v1/search?q=...&mode=hybrid
                       ├── keyword search (OpenSearch BM25/multi_match)
                       ├── semantic search (OpenSearch kNN)
                       └── Reciprocal Rank Fusion → ranked results
```

---

## Quick Start

### Prerequisites

- Docker 24+ and Docker Compose v2
- 4 GB RAM minimum (8 GB recommended — NLP model is ~500 MB)

### 1. Clone and configure

```bash
git clone https://github.com/your-org/search-engine.git
cd search-engine

# Create your .env file
cp configs/.env.example .env
# Edit .env and add your API keys (see "Free API Keys" section below)
```

### 2. Start the stack

```bash
cd docker
docker compose up --build
```

First start downloads the NLP model (~500 MB). Subsequent starts are fast.

### 3. Verify services are running

```bash
curl http://localhost:8080/health
curl http://localhost:8000/health
curl http://localhost:8001/health
curl http://localhost:8081/health
```

### 4. Seed some data

```bash
# Crawl documents from enabled APIs
curl -X POST http://localhost:8000/crawl \
  -H "Content-Type: application/json" \
  -d '{"query": "machine learning", "sources": ["github", "stackoverflow"], "limit": 5}'
```

Documents flow automatically:  
`crawler → Redis → NLP → Redis → Indexer → OpenSearch`

### 5. Search

```bash
# Hybrid search (default)
curl "http://localhost:8080/api/v1/search?q=machine+learning"

# Keyword only (BM25)
curl "http://localhost:8080/api/v1/search?q=neural+networks&mode=keyword"

# Semantic only (vector similarity)
curl "http://localhost:8080/api/v1/search?q=deep+learning+transformers&mode=semantic"

# With filters
curl "http://localhost:8080/api/v1/search?q=golang&source=github&page=2&page_size=5"
```

---

## API Reference

### Search API  (`go-api` — port 8080)

#### `GET /api/v1/search`

| Parameter   | Type   | Default   | Description                                  |
|-------------|--------|-----------|----------------------------------------------|
| `q`         | string | required  | Search query                                 |
| `mode`      | string | `hybrid`  | `keyword` / `semantic` / `hybrid`            |
| `page`      | int    | `1`       | Page number                                  |
| `page_size` | int    | `10`      | Results per page (max 50)                    |
| `source`    | string |           | Filter by source (`github`, `newsapi`, etc.) |
| `language`  | string |           | Filter by language (`en`, `fr`, etc.)        |
| `date_from` | string |           | ISO date filter from                         |
| `date_to`   | string |           | ISO date filter to                           |
| `min_score` | float  | `0`       | Minimum relevance score                      |

**Response:**
```json
{
  "query": "machine learning",
  "total": 42,
  "page": 1,
  "page_size": 10,
  "total_pages": 5,
  "mode": "hybrid",
  "duration_ms": 48,
  "hits": [
    {
      "id": "a3f9b1c2d4e5",
      "url": "https://github.com/example/ml-repo",
      "title": "Machine Learning Framework",
      "snippet": "…a flexible machine learning framework for production use…",
      "score": 0.0312,
      "keyword_score": 8.42,
      "semantic_score": 0.91,
      "source": "github",
      "published_date": "2024-03-15T00:00:00Z",
      "keyphrases": ["machine learning", "neural networks"]
    }
  ]
}
```

#### `GET /api/v1/document/:id`

Retrieve a full document by its ID.

#### `GET /health`

Service health check including downstream dependencies.

#### `GET /metrics`

Prometheus metrics endpoint.

---

### Crawler API  (`python-crawler` — port 8000)

#### `POST /crawl`
```json
{
  "query": "golang search engine",
  "sources": ["tavily", "github", "stackoverflow", "newsapi"],
  "limit": 10,
  "crawl_urls": true
}
```

#### `POST /crawl/urls`
```json
{
  "urls": ["https://example.com", "https://example.org"],
  "concurrency": 5
}
```

#### `GET /stats`
Returns queue depths and visited URL count.

---

### NLP API  (`python-nlp` — port 8001)

#### `POST /process`
```json
{
  "documents": [
    {"url": "https://example.com", "title": "Example", "content": "Raw text..."}
  ]
}
```

#### `POST /embed`
```json
{"text": "your query or document text"}
```
Returns `{"embedding": [...384 floats...], "dimension": 384}`

---

### Indexer API  (`go-indexer` — port 8081)

#### `POST /index`
Directly index pre-processed documents (bypasses queue):
```json
{
  "documents": [
    {
      "id": "abc123",
      "url": "https://example.com",
      "title": "Example Document",
      "content": "Document content...",
      "tokens": ["document", "content"],
      "embedding": [0.1, 0.2, ...],
      "word_count": 2,
      "language": "en",
      "source": "manual"
    }
  ]
}
```

---

## Free API Keys

All external data sources use free tiers — no credit card required:

| Service         | Free Tier                  | Sign-up URL                        |
|-----------------|----------------------------|------------------------------------|
| **Tavily**      | 1,000 req/month            | https://tavily.com                 |
| **Exa**         | 1,000 req/month            | https://exa.ai                     |
| **Serper.dev**  | 2,500 queries free         | https://serper.dev                 |
| **NewsAPI**     | 100 req/day (developer)    | https://newsapi.org                |
| **GitHub**      | 5,000 req/hr (with token)  | https://github.com/settings/tokens |
| **StackExchange**| No key needed (60 req/hr) | https://stackapps.com              |
| **Unsplash**    | 50 req/hr (demo)           | https://unsplash.com/developers    |

The system works with **zero API keys** — GitHub and StackOverflow work unauthenticated. Add keys to unlock higher rate limits.

---

## Project Structure

```
search-engine/
├── services/
│   ├── python-crawler/          # Data ingestion service
│   │   ├── crawler/             # BS4, Playwright, robots.txt
│   │   ├── sources/             # One file per API source
│   │   ├── tests/
│   │   ├── main.py              # FastAPI app
│   │   ├── requirements.txt
│   │   └── Dockerfile
│   │
│   ├── python-nlp/              # NLP processing service
│   │   ├── nlp/                 # cleaner, tokenizer, embedder, NER
│   │   ├── tests/
│   │   ├── main.py              # FastAPI app + queue worker
│   │   ├── requirements.txt
│   │   └── Dockerfile
│   │
│   ├── go-indexer/              # Document indexing service
│   │   ├── internal/
│   │   │   ├── indexer/         # OpenSearch bulk indexing
│   │   │   ├── database/        # Postgres pool + migrations
│   │   │   ├── models/          # Shared data types
│   │   │   └── queue/           # Redis consumer
│   │   ├── tests/
│   │   ├── main.go
│   │   ├── go.mod
│   │   └── Dockerfile
│   │
│   └── go-api/                  # Search API service
│       ├── internal/
│       │   ├── handlers/        # search, document, health
│       │   ├── ranking/         # BM25, semantic, hybrid RRF
│       │   ├── middleware/      # rate limiting, logging, recovery
│       │   ├── cache/           # Redis result cache
│       │   └── models/          # Request/response types
│       ├── tests/
│       ├── main.go
│       ├── go.mod
│       └── Dockerfile
│
├── shared/
│   ├── configs/
│   │   └── prometheus.yml
│   └── scripts/
│       ├── init_db.sql          # Postgres schema
│       ├── setup.sh             # First-run setup
│       └── integration_test.sh  # Live stack smoke tests
│
├── configs/
│   └── .env.example             # All configurable variables
│
└── docker/
    └── docker-compose.yml       # Full stack definition
```

---

## Running Tests

### Go unit tests
```bash
cd services/go-api
go test ./tests/... -v

cd services/go-indexer
go test ./tests/... -v
```

### Python unit tests
```bash
cd services/python-nlp
pip install pytest pytest-asyncio
pytest tests/ -v

cd services/python-crawler
pytest tests/ -v
```

### Integration tests (requires running stack)
```bash
bash shared/scripts/integration_test.sh
```

---

## Optional: Monitoring Stack

```bash
cd docker
docker compose --profile monitoring up -d
```

- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000 (admin / admin)

## Optional: OpenSearch Dashboards

```bash
cd docker
docker compose --profile dashboards up -d
```

- **OpenSearch Dashboards**: http://localhost:5601

---

## Configuration Reference

All settings are controlled via environment variables (see `configs/.env.example`).

| Variable            | Default                    | Description                         |
|---------------------|----------------------------|-------------------------------------|
| `EMBEDDING_MODEL`   | `all-MiniLM-L6-v2`         | Sentence-Transformers model name    |
| `NLP_BATCH_SIZE`    | `16`                       | Documents per embedding batch       |
| `MAX_TEXT_CHARS`    | `8000`                     | Max chars per document for NLP      |
| `RATE_RPS`          | `20`                       | API rate limit (requests/second/IP) |
| `RATE_BURST`        | `50`                       | Rate limit burst size               |
| `USE_PLAYWRIGHT`    | `false`                    | Enable JS-rendered page crawling    |

---

## Production Notes

- The NLP service runs a background queue worker consuming `queue:crawled` and producing `queue:processed`. No additional orchestration needed.
- OpenSearch is configured with `knn_vector` mapping (384-dim) for semantic search. Changing the embedding model requires re-indexing.
- Redis is used for three purposes: work queues, visited URL deduplication (SET), and search result caching (JSON with 5-min TTL).
- BM25 ranking within OpenSearch is augmented by field boosting (`title^3`, `keyphrases^2`, `content^1`).
- Hybrid search uses **Reciprocal Rank Fusion** (k=60), which is robust without requiring score normalisation.
