"""Exa (formerly Metaphor) neural search API source."""
from __future__ import annotations

from typing import Any

import structlog

from .base_source import BaseSource

logger = structlog.get_logger(__name__)

EXA_SEARCH_URL = "https://api.exa.ai/search"
EXA_CONTENTS_URL = "https://api.exa.ai/contents"


class ExaSource(BaseSource):
    SOURCE_NAME = "exa"

    async def fetch(self, query: str, limit: int = 10) -> list[dict[str, Any]]:
        if not self.api_key:
            logger.warning("exa_key_missing")
            return []
        headers = {"x-api-key": self.api_key, "Content-Type": "application/json"}
        payload = {
            "query": query,
            "numResults": min(limit, 10),
            "type": "neural",
            "useAutoprompt": True,
            "contents": {"text": True},
        }
        try:
            resp = await self._post(EXA_SEARCH_URL, json=payload, headers=headers)
            data = resp.json()
            return [
                {
                    "url": r.get("url", ""),
                    "title": r.get("title", ""),
                    "content": (r.get("text") or r.get("highlights", [""])[0] if r.get("highlights") else ""),
                    "score": r.get("score", 0.0),
                    "source": self.SOURCE_NAME,
                    "published_date": r.get("publishedDate"),
                    "author": r.get("author"),
                }
                for r in data.get("results", [])
            ]
        except Exception as exc:
            logger.error("exa_fetch_error", error=str(exc), query=query)
            return []
