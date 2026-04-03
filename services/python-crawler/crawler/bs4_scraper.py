"""BeautifulSoup4 scraper for static HTML pages."""
from __future__ import annotations

import re
from typing import Any
from urllib.parse import urljoin, urlparse

import httpx
import structlog
from bs4 import BeautifulSoup
from fake_useragent import UserAgent
from tenacity import retry, stop_after_attempt, wait_exponential

from .robots_checker import RobotsChecker

logger = structlog.get_logger(__name__)

ua = UserAgent()


class BS4Scraper:
    def __init__(self, robots_checker: RobotsChecker | None = None) -> None:
        self._robots = robots_checker or RobotsChecker()

    @retry(stop=stop_after_attempt(3), wait=wait_exponential(multiplier=1, min=2, max=8))
    async def scrape(self, url: str) -> dict[str, Any] | None:
        if not await self._robots.can_fetch(url):
            return None
        headers = {"User-Agent": ua.random}
        try:
            async with httpx.AsyncClient(timeout=20, follow_redirects=True) as client:
                resp = await client.get(url, headers=headers)
                resp.raise_for_status()
                content_type = resp.headers.get("content-type", "")
                if "text/html" not in content_type:
                    return None
                return self._parse(url, resp.text)
        except Exception as exc:
            logger.warning("bs4_scrape_failed", url=url, error=str(exc))
            return None

    def _parse(self, url: str, html: str) -> dict[str, Any]:
        soup = BeautifulSoup(html, "lxml")

        # Remove noise elements
        for tag in soup(["script", "style", "nav", "footer", "header", "aside", "iframe"]):
            tag.decompose()

        title = ""
        if soup.title:
            title = soup.title.get_text(strip=True)

        meta_desc = ""
        meta = soup.find("meta", attrs={"name": "description"})
        if meta and isinstance(meta, object) and hasattr(meta, "get"):
            meta_desc = meta.get("content", "")  # type: ignore[union-attr]

        # Extract main content
        main = soup.find("main") or soup.find("article") or soup.find("body")
        if main:
            raw_text = main.get_text(separator=" ", strip=True)
        else:
            raw_text = soup.get_text(separator=" ", strip=True)

        text = re.sub(r"\s+", " ", raw_text).strip()

        links = [
            urljoin(url, a["href"])
            for a in soup.find_all("a", href=True)
            if urlparse(urljoin(url, a["href"])).scheme in ("http", "https")
        ][:50]

        return {
            "url": url,
            "title": title,
            "meta_description": meta_desc,
            "content": text[:10000],
            "links": links,
            "source": "bs4_scraper",
        }
