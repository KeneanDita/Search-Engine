"""Base class for all data sources."""
from __future__ import annotations

import abc
from typing import Any

import httpx
import structlog
from tenacity import retry, stop_after_attempt, wait_exponential, retry_if_exception_type

logger = structlog.get_logger(__name__)


class BaseSource(abc.ABC):
    """Abstract base for every external data source."""

    SOURCE_NAME: str = "base"

    def __init__(self, api_key: str | None = None, timeout: int = 30) -> None:
        self.api_key = api_key
        self._client = httpx.AsyncClient(
            timeout=httpx.Timeout(timeout),
            headers=self._default_headers(),
        )

    def _default_headers(self) -> dict[str, str]:
        return {"User-Agent": "SearchEngine/1.0 (research bot; respects robots.txt)"}

    @retry(
        stop=stop_after_attempt(3),
        wait=wait_exponential(multiplier=1, min=2, max=10),
        retry=retry_if_exception_type((httpx.TimeoutException, httpx.NetworkError)),
    )
    async def _get(self, url: str, **kwargs: Any) -> httpx.Response:
        logger.debug("http_get", source=self.SOURCE_NAME, url=url)
        response = await self._client.get(url, **kwargs)
        response.raise_for_status()
        return response

    @retry(
        stop=stop_after_attempt(3),
        wait=wait_exponential(multiplier=1, min=2, max=10),
        retry=retry_if_exception_type((httpx.TimeoutException, httpx.NetworkError)),
    )
    async def _post(self, url: str, **kwargs: Any) -> httpx.Response:
        logger.debug("http_post", source=self.SOURCE_NAME, url=url)
        response = await self._client.post(url, **kwargs)
        response.raise_for_status()
        return response

    @abc.abstractmethod
    async def fetch(self, query: str, limit: int = 10) -> list[dict[str, Any]]:
        """Fetch documents matching query. Returns list of raw document dicts."""

    async def close(self) -> None:
        await self._client.aclose()

    async def __aenter__(self) -> "BaseSource":
        return self

    async def __aexit__(self, *_: Any) -> None:
        await self.close()
