"""Python NLP Service — FastAPI entrypoint."""
from __future__ import annotations

import asyncio
import json
import os
import time
from concurrent.futures import ThreadPoolExecutor
from typing import Any

import redis.asyncio as aioredis
import structlog
from fastapi import FastAPI, HTTPException
from prometheus_client import Counter, Histogram, generate_latest, CONTENT_TYPE_LATEST
from pydantic import BaseModel
from starlette.responses import Response

from nlp.text_cleaner import TextCleaner
from nlp.tokenizer import Tokenizer
from nlp.embedder import Embedder
from nlp.ner import NERExtractor

# ── Logging ────────────────────────────────────────────────────────────────
structlog.configure(
    processors=[
        structlog.processors.TimeStamper(fmt="iso"),
        structlog.processors.add_log_level,
        structlog.processors.JSONRenderer(),
    ]
)
logger = structlog.get_logger(__name__)

# ── Settings ───────────────────────────────────────────────────────────────
REDIS_URL = os.getenv("REDIS_URL", "redis://redis:6379")
INDEXER_URL = os.getenv("INDEXER_URL", "http://go-indexer:8081")
CRAWLED_QUEUE = "queue:crawled"
PROCESSED_QUEUE = "queue:processed"
BATCH_SIZE = int(os.getenv("NLP_BATCH_SIZE", "16"))
MAX_TEXT_CHARS = int(os.getenv("MAX_TEXT_CHARS", "8000"))

# ── Metrics ────────────────────────────────────────────────────────────────
DOCS_PROCESSED = Counter("nlp_docs_processed_total", "Total documents processed")
PROCESSING_DURATION = Histogram("nlp_processing_seconds", "Document processing time")
EMBED_DURATION = Histogram("nlp_embed_seconds", "Embedding generation time")

# ── App ────────────────────────────────────────────────────────────────────
app = FastAPI(title="Search Engine NLP Service", version="1.0.0")

cleaner = TextCleaner()
tokenizer = Tokenizer()
embedder = Embedder()
ner = NERExtractor()

_executor = ThreadPoolExecutor(max_workers=4)
_worker_task: asyncio.Task | None = None
_redis: aioredis.Redis | None = None


@app.on_event("startup")
async def startup() -> None:
    global _redis, _worker_task
    _redis = aioredis.from_url(REDIS_URL, decode_responses=True)
    # Pre-warm the embedding model
    loop = asyncio.get_event_loop()
    await loop.run_in_executor(_executor, embedder.embed, "warmup")
    _worker_task = asyncio.create_task(_queue_worker())
    logger.info("nlp_service_started")


@app.on_event("shutdown")
async def shutdown() -> None:
    if _worker_task:
        _worker_task.cancel()
    if _redis:
        await _redis.aclose()


# ── Pydantic Models ────────────────────────────────────────────────────────
class ProcessRequest(BaseModel):
    documents: list[dict[str, Any]]


class ProcessedDocument(BaseModel):
    id: str
    url: str
    title: str
    content: str
    tokens: list[str]
    keyphrases: list[str]
    entities: dict[str, list[str]]
    embedding: list[float]
    word_count: int
    language: str = "en"
    source: str = ""
    published_date: str | None = None
    metadata: dict[str, Any] = {}


# ── Core processing ────────────────────────────────────────────────────────
def _process_document_sync(raw: dict[str, Any]) -> dict[str, Any]:
    """CPU-bound NLP processing — runs in thread executor."""
    start = time.perf_counter()

    url = raw.get("url", "")
    title = raw.get("title", "")
    raw_content = raw.get("content") or raw.get("raw_content") or ""

    # Clean
    if raw.get("_is_html"):
        text = cleaner.clean_html(raw_content)
    else:
        text = cleaner.clean_text(raw_content)

    text = cleaner.truncate(text, MAX_TEXT_CHARS)
    full_text = f"{title} {text}".strip()

    tokens = tokenizer.tokenize(full_text)
    keyphrases = tokenizer.keyphrases(full_text)
    entities = ner.extract(full_text)

    embed_start = time.perf_counter()
    embedding = embedder.embed(full_text[:512])
    EMBED_DURATION.observe(time.perf_counter() - embed_start)

    import hashlib
    doc_id = hashlib.sha256(url.encode()).hexdigest()[:16]

    DOCS_PROCESSED.inc()
    PROCESSING_DURATION.observe(time.perf_counter() - start)

    return {
        "id": doc_id,
        "url": url,
        "title": title,
        "content": text,
        "tokens": tokens[:500],
        "keyphrases": keyphrases,
        "entities": entities,
        "embedding": embedding,
        "word_count": len(tokens),
        "language": "en",
        "source": raw.get("source", ""),
        "published_date": raw.get("published_date"),
        "metadata": {
            k: v for k, v in raw.items()
            if k not in ("content", "raw_content", "url", "title", "source", "published_date")
        },
    }


# ── Queue worker ───────────────────────────────────────────────────────────
async def _queue_worker() -> None:
    """Continuously drain the crawled queue, process, and re-enqueue."""
    logger.info("queue_worker_started", input_queue=CRAWLED_QUEUE)
    loop = asyncio.get_event_loop()
    while True:
        try:
            item = await _redis.blpop(CRAWLED_QUEUE, timeout=5)  # type: ignore[union-attr]
            if not item:
                continue
            _, payload = item
            raw_doc = json.loads(payload)
            processed = await loop.run_in_executor(_executor, _process_document_sync, raw_doc)
            await _redis.rpush(PROCESSED_QUEUE, json.dumps(processed))  # type: ignore[union-attr]
            logger.debug("doc_processed", doc_id=processed["id"], url=processed["url"])
        except asyncio.CancelledError:
            break
        except Exception as exc:
            logger.error("queue_worker_error", error=str(exc))
            await asyncio.sleep(1)


# ── Routes ─────────────────────────────────────────────────────────────────
@app.get("/health")
async def health() -> dict[str, str]:
    return {"status": "ok", "service": "python-nlp"}


@app.get("/metrics")
async def metrics() -> Response:
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)


@app.post("/process")
async def process_documents(req: ProcessRequest) -> dict[str, Any]:
    """Synchronously process a batch of documents and return results."""
    loop = asyncio.get_event_loop()
    results = []
    texts_for_batch = []
    raws = []

    for doc in req.documents:
        url = doc.get("url", "")
        title = doc.get("title", "")
        raw_content = doc.get("content") or ""
        text = cleaner.clean_text(raw_content)
        text = cleaner.truncate(text, MAX_TEXT_CHARS)
        texts_for_batch.append(f"{title} {text}".strip())
        raws.append((doc, text))

    # Batch embed all at once
    embeddings = await loop.run_in_executor(
        _executor, embedder.embed_batch, texts_for_batch
    )

    for i, (raw_doc, text) in enumerate(raws):
        full_text = texts_for_batch[i]
        tokens = await loop.run_in_executor(_executor, tokenizer.tokenize, full_text)
        keyphrases = await loop.run_in_executor(_executor, tokenizer.keyphrases, full_text)
        entities = await loop.run_in_executor(_executor, ner.extract, full_text)

        import hashlib
        doc_id = hashlib.sha256(raw_doc.get("url", str(i)).encode()).hexdigest()[:16]

        results.append({
            "id": doc_id,
            "url": raw_doc.get("url", ""),
            "title": raw_doc.get("title", ""),
            "content": text,
            "tokens": tokens[:500],
            "keyphrases": keyphrases,
            "entities": entities,
            "embedding": embeddings[i],
            "word_count": len(tokens),
            "source": raw_doc.get("source", ""),
            "published_date": raw_doc.get("published_date"),
            "metadata": {},
        })

    return {"processed": len(results), "documents": results}


@app.post("/embed")
async def embed_text(body: dict[str, Any]) -> dict[str, Any]:
    """Embed a single query or document text."""
    text = body.get("text", "")
    if not text:
        raise HTTPException(status_code=400, detail="text field required")
    loop = asyncio.get_event_loop()
    vec = await loop.run_in_executor(_executor, embedder.embed, text[:512])
    return {"embedding": vec, "dimension": len(vec)}


@app.get("/stats")
async def stats() -> dict[str, Any]:
    queued_crawled = await _redis.llen(CRAWLED_QUEUE)  # type: ignore[union-attr]
    queued_processed = await _redis.llen(PROCESSED_QUEUE)  # type: ignore[union-attr]
    return {
        "queue_crawled": queued_crawled,
        "queue_processed": queued_processed,
        "embedding_model": embedder._model_name,
        "embedding_dim": embedder.dimension,
    }
