"""Tavily Search API source (free tier: 1000 req/month)."""
from __future__ import annotations

from typing import Any

import structlog

from .base_source import BaseSource

logger = structlog.get_logger(__name__)

TAVILY_API_URL = "https://api.tavily.com/search"


class TavilySource(BaseSource):
    SOURCE_NAME = "tavily"

    async def fetch(self, query: str, limit: int = 10) -> list[dict[str, Any]]:
        if not self.api_key:
            logger.warning("tavily_key_missing")
            return []
        payload = {
            "api_key": self.api_key,
            "query": query,
            "search_depth": "basic",
            "max_results": min(limit, 20),
            "include_raw_content": True,
        }
        try:
            resp = await self._post(TAVILY_API_URL, json=payload)
            data = resp.json()
            return [
                {
                    "url": r.get("url", ""),
                    "title": r.get("title", ""),
                    "content": r.get("content", ""),
                    "raw_content": r.get("raw_content", ""),
                    "score": r.get("score", 0.0),
                    "source": self.SOURCE_NAME,
                    "published_date": r.get("published_date"),
                }
                for r in data.get("results", [])
            ]
        except Exception as exc:
            logger.error("tavily_fetch_error", error=str(exc), query=query)
            return []
