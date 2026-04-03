"""Serper.dev SERP API source (free tier: 2500 queries)."""
from __future__ import annotations

from typing import Any

import structlog

from .base_source import BaseSource

logger = structlog.get_logger(__name__)

SERPER_URL = "https://google.serper.dev/search"


class SerperSource(BaseSource):
    SOURCE_NAME = "serper"

    async def fetch(self, query: str, limit: int = 10) -> list[dict[str, Any]]:
        if not self.api_key:
            logger.warning("serper_key_missing")
            return []
        headers = {"X-API-KEY": self.api_key, "Content-Type": "application/json"}
        payload = {"q": query, "num": min(limit, 10)}
        try:
            resp = await self._post(SERPER_URL, json=payload, headers=headers)
            data = resp.json()
            results = []
            for r in data.get("organic", []):
                results.append({
                    "url": r.get("link", ""),
                    "title": r.get("title", ""),
                    "content": r.get("snippet", ""),
                    "score": 1.0 / (r.get("position", 10) + 1),
                    "source": self.SOURCE_NAME,
                    "published_date": r.get("date"),
                })
            return results[:limit]
        except Exception as exc:
            logger.error("serper_fetch_error", error=str(exc), query=query)
            return []
