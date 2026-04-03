"""Unsplash API source for image content (free: 50 req/hour)."""
from __future__ import annotations

from typing import Any

import structlog

from .base_source import BaseSource

logger = structlog.get_logger(__name__)

UNSPLASH_URL = "https://api.unsplash.com/search/photos"


class UnsplashSource(BaseSource):
    SOURCE_NAME = "unsplash"

    async def fetch(self, query: str, limit: int = 10) -> list[dict[str, Any]]:
        if not self.api_key:
            logger.warning("unsplash_key_missing")
            return []
        params = {
            "query": query,
            "per_page": min(limit, 30),
            "order_by": "relevant",
        }
        headers = {"Authorization": f"Client-ID {self.api_key}"}
        try:
            resp = await self._get(UNSPLASH_URL, params=params, headers=headers)
            data = resp.json()
            return [
                {
                    "url": p.get("links", {}).get("html", ""),
                    "title": p.get("alt_description") or p.get("description") or query,
                    "content": p.get("description") or "",
                    "score": p.get("likes", 0) / 1000,
                    "source": self.SOURCE_NAME,
                    "published_date": p.get("created_at"),
                    "image_url": p.get("urls", {}).get("regular", ""),
                    "author": p.get("user", {}).get("name", ""),
                    "content_type": "image",
                }
                for p in data.get("results", [])
            ]
        except Exception as exc:
            logger.error("unsplash_fetch_error", error=str(exc), query=query)
            return []
