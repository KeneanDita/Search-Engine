"""Python Crawler Service — FastAPI entrypoint."""
from __future__ import annotations

import asyncio
import os
from typing import Any

import structlog
from fastapi import FastAPI, BackgroundTasks, HTTPException
from fastapi.responses import JSONResponse
from prometheus_client import Counter, Histogram, generate_latest, CONTENT_TYPE_LATEST
from pydantic import BaseModel
from starlette.responses import Response

from crawler.pipeline import CrawlPipeline
from sources import (
    TavilySource, ExaSource, SerperSource,
    NewsAPISource, GitHubSource, StackOverflowSource, UnsplashSource,
)

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
USE_PLAYWRIGHT = os.getenv("USE_PLAYWRIGHT", "false").lower() == "true"

# ── Metrics ────────────────────────────────────────────────────────────────
CRAWL_REQUESTS = Counter("crawler_requests_total", "Total crawl requests", ["source"])
CRAWL_DURATION = Histogram("crawler_duration_seconds", "Crawl request duration")
DOCS_FETCHED = Counter("crawler_docs_fetched_total", "Documents fetched", ["source"])

# ── App ────────────────────────────────────────────────────────────────────
app = FastAPI(title="Search Engine Crawler", version="1.0.0")
pipeline: CrawlPipeline | None = None


@app.on_event("startup")
async def startup() -> None:
    global pipeline
    pipeline = CrawlPipeline(redis_url=REDIS_URL, use_playwright=USE_PLAYWRIGHT)
    await pipeline.start()
    logger.info("crawler_service_started", redis=REDIS_URL)


@app.on_event("shutdown")
async def shutdown() -> None:
    if pipeline:
        await pipeline.stop()


# ── Pydantic Models ────────────────────────────────────────────────────────
class CrawlRequest(BaseModel):
    query: str
    sources: list[str] = ["tavily", "serper", "newsapi", "github", "stackoverflow"]
    limit: int = 10
    crawl_urls: bool = True


class CrawlURLRequest(BaseModel):
    urls: list[str]
    concurrency: int = 5


# ── Source registry ────────────────────────────────────────────────────────
def _build_sources() -> dict[str, Any]:
    return {
        "tavily": TavilySource(api_key=os.getenv("TAVILY_API_KEY")),
        "exa": ExaSource(api_key=os.getenv("EXA_API_KEY")),
        "serper": SerperSource(api_key=os.getenv("SERPER_API_KEY")),
        "newsapi": NewsAPISource(api_key=os.getenv("NEWSAPI_KEY")),
        "github": GitHubSource(api_key=os.getenv("GITHUB_TOKEN")),
        "stackoverflow": StackOverflowSource(api_key=os.getenv("STACKEXCHANGE_KEY")),
        "unsplash": UnsplashSource(api_key=os.getenv("UNSPLASH_ACCESS_KEY")),
    }


# ── Routes ─────────────────────────────────────────────────────────────────
@app.get("/health")
async def health() -> dict[str, str]:
    return {"status": "ok", "service": "python-crawler"}


@app.get("/metrics")
async def metrics() -> Response:
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)


@app.post("/crawl")
async def crawl(req: CrawlRequest, background_tasks: BackgroundTasks) -> dict[str, Any]:
    """Fetch seed URLs from APIs, then optionally deep-scrape them."""
    source_registry = _build_sources()
    all_docs: list[dict[str, Any]] = []
    urls_to_crawl: list[str] = []

    for source_name in req.sources:
        src = source_registry.get(source_name)
        if not src:
            continue
        CRAWL_REQUESTS.labels(source=source_name).inc()
        try:
            docs = await src.fetch(req.query, limit=req.limit)
            DOCS_FETCHED.labels(source=source_name).inc(len(docs))
            all_docs.extend(docs)
            urls_to_crawl.extend(d["url"] for d in docs if d.get("url"))
        except Exception as exc:
            logger.error("source_fetch_failed", source=source_name, error=str(exc))
        finally:
            await src.close()

    if req.crawl_urls and pipeline and urls_to_crawl:
        background_tasks.add_task(pipeline.crawl_batch, urls_to_crawl[:50])

    return {"query": req.query, "total": len(all_docs), "documents": all_docs}


@app.post("/crawl/urls")
async def crawl_urls(req: CrawlURLRequest) -> dict[str, Any]:
    """Directly crawl a list of URLs."""
    if not pipeline:
        raise HTTPException(status_code=503, detail="Pipeline not ready")
    count = await pipeline.crawl_batch(req.urls, concurrency=req.concurrency)
    return {"queued": count, "total_urls": len(req.urls)}


@app.get("/stats")
async def stats() -> dict[str, Any]:
    """Return basic queue stats."""
    import redis.asyncio as aioredis
    r = aioredis.from_url(REDIS_URL, decode_responses=True)
    try:
        queued = await r.llen("queue:crawled")
        visited = await r.scard("visited:urls")
        return {"queued_documents": queued, "visited_urls": visited}
    finally:
        await r.aclose()
