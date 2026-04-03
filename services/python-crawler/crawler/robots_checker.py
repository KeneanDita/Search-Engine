"""Robots.txt compliance checker with caching."""
from __future__ import annotations

import urllib.robotparser
from functools import lru_cache
from urllib.parse import urlparse

import httpx
import structlog

logger = structlog.get_logger(__name__)

BOT_NAME = "SearchEngineBot"


class RobotsChecker:
    def __init__(self) -> None:
        self._cache: dict[str, urllib.robotparser.RobotFileParser] = {}

    def _get_robots_url(self, url: str) -> str:
        parsed = urlparse(url)
        return f"{parsed.scheme}://{parsed.netloc}/robots.txt"

    async def can_fetch(self, url: str) -> bool:
        robots_url = self._get_robots_url(url)
        if robots_url not in self._cache:
            rp = urllib.robotparser.RobotFileParser()
            rp.set_url(robots_url)
            try:
                async with httpx.AsyncClient(timeout=5) as client:
                    resp = await client.get(robots_url)
                    rp.parse(resp.text.splitlines())
            except Exception:
                # If robots.txt is unreachable, allow crawling
                rp.allow_all = True
            self._cache[robots_url] = rp
        result: bool = self._cache[robots_url].can_fetch(BOT_NAME, url)
        if not result:
            logger.info("robots_disallowed", url=url)
        return result
