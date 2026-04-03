"""Unit tests for the crawler service."""
from __future__ import annotations

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from crawler.bs4_scraper import BS4Scraper
from crawler.robots_checker import RobotsChecker


@pytest.fixture
def robots_checker_allow():
    checker = RobotsChecker()
    checker.can_fetch = AsyncMock(return_value=True)
    return checker


@pytest.mark.asyncio
async def test_bs4_scraper_returns_doc(robots_checker_allow):
    html = """
    <html><head><title>Test Page</title></head>
    <body><main><p>Hello world content here.</p></main></body></html>
    """
    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.headers = {"content-type": "text/html"}
    mock_resp.text = html
    mock_resp.raise_for_status = MagicMock()

    scraper = BS4Scraper(robots_checker=robots_checker_allow)
    with patch("httpx.AsyncClient") as mock_client_cls:
        mock_client = AsyncMock()
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)
        mock_client.get = AsyncMock(return_value=mock_resp)
        mock_client_cls.return_value = mock_client

        doc = await scraper.scrape("https://example.com")

    assert doc is not None
    assert doc["title"] == "Test Page"
    assert "Hello world" in doc["content"]


@pytest.mark.asyncio
async def test_bs4_scraper_respects_robots():
    checker = RobotsChecker()
    checker.can_fetch = AsyncMock(return_value=False)
    scraper = BS4Scraper(robots_checker=checker)
    doc = await scraper.scrape("https://example.com/disallowed")
    assert doc is None


@pytest.mark.asyncio
async def test_tavily_source_no_key():
    from sources.tavily_source import TavilySource
    src = TavilySource(api_key=None)
    results = await src.fetch("test query")
    assert results == []


@pytest.mark.asyncio
async def test_github_source_fetch():
    from sources.github_source import GitHubSource
    mock_resp = MagicMock()
    mock_resp.json.return_value = {
        "items": [
            {
                "html_url": "https://github.com/test/repo",
                "full_name": "test/repo",
                "description": "A test repo",
                "stargazers_count": 500,
                "updated_at": "2024-01-01T00:00:00Z",
                "language": "Python",
                "topics": ["search", "nlp"],
            }
        ]
    }
    mock_resp.raise_for_status = MagicMock()

    src = GitHubSource(api_key="test-token")
    with patch.object(src, "_get", AsyncMock(return_value=mock_resp)):
        results = await src.fetch("search engine")

    assert len(results) == 1
    assert results[0]["url"] == "https://github.com/test/repo"
    assert results[0]["stars"] == 500
