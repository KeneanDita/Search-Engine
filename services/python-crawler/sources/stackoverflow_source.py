"""StackExchange API source (free, no key required for read)."""
from __future__ import annotations

from typing import Any

import structlog

from .base_source import BaseSource

logger = structlog.get_logger(__name__)

SE_SEARCH_URL = "https://api.stackexchange.com/2.3/search/advanced"


class StackOverflowSource(BaseSource):
    SOURCE_NAME = "stackoverflow"

    async def fetch(self, query: str, limit: int = 10) -> list[dict[str, Any]]:
        params = {
            "order": "desc",
            "sort": "relevance",
            "q": query,
            "site": "stackoverflow",
            "pagesize": min(limit, 30),
            "filter": "withbody",
        }
        if self.api_key:
            params["key"] = self.api_key
        try:
            resp = await self._get(SE_SEARCH_URL, params=params)
            data = resp.json()
            results = []
            for item in data.get("items", []):
                results.append({
                    "url": item.get("link", ""),
                    "title": item.get("title", ""),
                    "content": item.get("body", "")[:2000],
                    "score": min(1.0, item.get("score", 0) / 100),
                    "source": self.SOURCE_NAME,
                    "published_date": None,
                    "tags": item.get("tags", []),
                    "answer_count": item.get("answer_count", 0),
                    "is_answered": item.get("is_answered", False),
                })
            return results
        except Exception as exc:
            logger.error("stackoverflow_fetch_error", error=str(exc), query=query)
            return []
