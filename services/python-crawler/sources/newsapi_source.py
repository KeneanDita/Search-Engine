"""NewsAPI source (free tier: 100 req/day, developer plan)."""
from __future__ import annotations

from typing import Any

import structlog

from .base_source import BaseSource

logger = structlog.get_logger(__name__)

NEWSAPI_URL = "https://newsapi.org/v2/everything"


class NewsAPISource(BaseSource):
    SOURCE_NAME = "newsapi"

    async def fetch(self, query: str, limit: int = 10) -> list[dict[str, Any]]:
        if not self.api_key:
            logger.warning("newsapi_key_missing")
            return []
        params = {
            "q": query,
            "pageSize": min(limit, 100),
            "sortBy": "relevancy",
            "language": "en",
            "apiKey": self.api_key,
        }
        try:
            resp = await self._get(NEWSAPI_URL, params=params)
            data = resp.json()
            return [
                {
                    "url": a.get("url", ""),
                    "title": a.get("title", ""),
                    "content": (a.get("content") or a.get("description") or ""),
                    "score": 0.7,
                    "source": self.SOURCE_NAME,
                    "published_date": a.get("publishedAt"),
                    "author": a.get("author"),
                    "image_url": a.get("urlToImage"),
                }
                for a in data.get("articles", [])
                if a.get("url") and a.get("title")
            ]
        except Exception as exc:
            logger.error("newsapi_fetch_error", error=str(exc), query=query)
            return []
