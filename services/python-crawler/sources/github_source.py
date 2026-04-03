"""GitHub API source (free: 60 unauth / 5000 auth req/hour)."""
from __future__ import annotations

from typing import Any

import structlog

from .base_source import BaseSource

logger = structlog.get_logger(__name__)

GITHUB_SEARCH_URL = "https://api.github.com/search/repositories"
GITHUB_CODE_URL = "https://api.github.com/search/code"


class GitHubSource(BaseSource):
    SOURCE_NAME = "github"

    def _default_headers(self) -> dict[str, str]:
        headers: dict[str, str] = {
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
        }
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        return headers

    async def fetch(self, query: str, limit: int = 10) -> list[dict[str, Any]]:
        params = {"q": query, "sort": "stars", "order": "desc", "per_page": min(limit, 30)}
        try:
            resp = await self._get(GITHUB_SEARCH_URL, params=params)
            data = resp.json()
            return [
                {
                    "url": r.get("html_url", ""),
                    "title": r.get("full_name", ""),
                    "content": r.get("description") or "",
                    "score": min(1.0, (r.get("stargazers_count", 0) / 10000)),
                    "source": self.SOURCE_NAME,
                    "published_date": r.get("updated_at"),
                    "language": r.get("language"),
                    "stars": r.get("stargazers_count", 0),
                    "topics": r.get("topics", []),
                }
                for r in data.get("items", [])
            ]
        except Exception as exc:
            logger.error("github_fetch_error", error=str(exc), query=query)
            return []
