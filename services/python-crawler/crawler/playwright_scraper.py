"""Playwright scraper for JavaScript-heavy pages."""
from __future__ import annotations

import asyncio
import re
from typing import Any
from urllib.parse import urljoin, urlparse

import structlog
from playwright.async_api import async_playwright, TimeoutError as PlaywrightTimeout

from .robots_checker import RobotsChecker

logger = structlog.get_logger(__name__)


class PlaywrightScraper:
    """Uses headless Chromium via Playwright for JS-rendered content."""

    def __init__(self, robots_checker: RobotsChecker | None = None) -> None:
        self._robots = robots_checker or RobotsChecker()
        self._browser = None
        self._playwright = None

    async def start(self) -> None:
        self._playwright = await async_playwright().start()
        self._browser = await self._playwright.chromium.launch(
            headless=True,
            args=["--no-sandbox", "--disable-dev-shm-usage"],
        )

    async def stop(self) -> None:
        if self._browser:
            await self._browser.close()
        if self._playwright:
            await self._playwright.stop()

    async def scrape(self, url: str, wait_for: str = "networkidle") -> dict[str, Any] | None:
        if not await self._robots.can_fetch(url):
            return None
        if not self._browser:
            await self.start()

        try:
            context = await self._browser.new_context(  # type: ignore[union-attr]
                user_agent="Mozilla/5.0 (compatible; SearchEngineBot/1.0)"
            )
            page = await context.new_page()
            await page.goto(url, wait_until=wait_for, timeout=30000)
            html = await page.content()
            title = await page.title()
            await context.close()
            return self._parse(url, html, title)
        except PlaywrightTimeout:
            logger.warning("playwright_timeout", url=url)
            return None
        except Exception as exc:
            logger.warning("playwright_error", url=url, error=str(exc))
            return None

    def _parse(self, url: str, html: str, title: str) -> dict[str, Any]:
        from bs4 import BeautifulSoup

        soup = BeautifulSoup(html, "lxml")
        for tag in soup(["script", "style", "nav", "footer", "aside"]):
            tag.decompose()

        main = soup.find("main") or soup.find("article") or soup.find("body")
        raw_text = main.get_text(separator=" ", strip=True) if main else soup.get_text(separator=" ", strip=True)
        text = re.sub(r"\s+", " ", raw_text).strip()

        links = [
            urljoin(url, a["href"])
            for a in soup.find_all("a", href=True)
            if urlparse(urljoin(url, a["href"])).scheme in ("http", "https")
        ][:50]

        return {
            "url": url,
            "title": title,
            "content": text[:10000],
            "links": links,
            "source": "playwright_scraper",
        }

    async def __aenter__(self) -> "PlaywrightScraper":
        await self.start()
        return self

    async def __aexit__(self, *_: Any) -> None:
        await self.stop()
