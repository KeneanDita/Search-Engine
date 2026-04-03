"""Crawl pipeline: fetch seeds → scrape → push to Redis queue."""
from __future__ import annotations

import asyncio
import hashlib
import json
import time
from typing import Any

import redis.asyncio as aioredis
import structlog

from .bs4_scraper import BS4Scraper
from .playwright_scraper import PlaywrightScraper
from .robots_checker import RobotsChecker

logger = structlog.get_logger(__name__)

CRAWLED_QUEUE = "queue:crawled"
VISITED_SET = "visited:urls"


class CrawlPipeline:
    def __init__(self, redis_url: str, use_playwright: bool = False) -> None:
        self._redis_url = redis_url
        self._use_playwright = use_playwright
        self._robots = RobotsChecker()
        self._bs4 = BS4Scraper(robots_checker=self._robots)
        self._playwright: PlaywrightScraper | None = None
        self._redis: aioredis.Redis | None = None

    async def start(self) -> None:
        self._redis = aioredis.from_url(self._redis_url, decode_responses=True)
        if self._use_playwright:
            self._playwright = PlaywrightScraper(robots_checker=self._robots)
            await self._playwright.start()

    async def stop(self) -> None:
        if self._playwright:
            await self._playwright.stop()
        if self._redis:
            await self._redis.aclose()

    def _url_hash(self, url: str) -> str:
        return hashlib.sha256(url.encode()).hexdigest()

    async def _already_visited(self, url: str) -> bool:
        return bool(await self._redis.sismember(VISITED_SET, self._url_hash(url)))  # type: ignore[union-attr]

    async def _mark_visited(self, url: str) -> None:
        await self._redis.sadd(VISITED_SET, self._url_hash(url))  # type: ignore[union-attr]

    async def _push_document(self, doc: dict[str, Any]) -> None:
        doc["crawled_at"] = time.time()
        await self._redis.rpush(CRAWLED_QUEUE, json.dumps(doc))  # type: ignore[union-attr]
        logger.info("document_queued", url=doc.get("url"), queue=CRAWLED_QUEUE)

    async def crawl_url(self, url: str) -> dict[str, Any] | None:
        if await self._already_visited(url):
            logger.debug("url_already_visited", url=url)
            return None

        doc: dict[str, Any] | None = None

        if self._use_playwright and self._playwright:
            doc = await self._playwright.scrape(url)

        if doc is None:
            doc = await self._bs4.scrape(url)

        if doc:
            await self._mark_visited(url)
            await self._push_document(doc)

        return doc

    async def crawl_batch(self, urls: list[str], concurrency: int = 5) -> int:
        semaphore = asyncio.Semaphore(concurrency)
        count = 0

        async def _crawl_one(url: str) -> None:
            nonlocal count
            async with semaphore:
                result = await self.crawl_url(url)
                if result:
                    count += 1

        await asyncio.gather(*[_crawl_one(url) for url in urls])
        return count

    async def __aenter__(self) -> "CrawlPipeline":
        await self.start()
        return self

    async def __aexit__(self, *_: Any) -> None:
        await self.stop()
