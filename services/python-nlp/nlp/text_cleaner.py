"""HTML-to-clean-text pipeline."""
from __future__ import annotations

import re
import unicodedata

import bleach
import structlog
from bs4 import BeautifulSoup

logger = structlog.get_logger(__name__)

# Allowed tags during bleach cleaning (strip everything for plain text)
_ALLOWED_TAGS: list[str] = []


class TextCleaner:
    """Cleans raw HTML or dirty text into normalised plain text."""

    def clean_html(self, html: str) -> str:
        """Strip HTML tags and return clean text."""
        try:
            soup = BeautifulSoup(html, "lxml")
            for tag in soup(["script", "style", "nav", "footer", "header", "aside"]):
                tag.decompose()
            text = soup.get_text(separator=" ", strip=True)
        except Exception:
            text = bleach.clean(html, tags=_ALLOWED_TAGS, strip=True)
        return self.clean_text(text)

    def clean_text(self, text: str) -> str:
        """Normalise unicode, remove control chars, collapse whitespace."""
        if not text:
            return ""
        # Unicode normalisation
        text = unicodedata.normalize("NFKC", text)
        # Remove control characters (except newlines/tabs)
        text = re.sub(r"[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]", "", text)
        # Collapse multiple spaces/newlines
        text = re.sub(r"[ \t]+", " ", text)
        text = re.sub(r"\n{3,}", "\n\n", text)
        # Remove URLs for NLP (keep for metadata)
        text = re.sub(r"https?://\S+", " ", text)
        # Remove very long tokens (likely base64 / hashes)
        text = re.sub(r"\b\S{60,}\b", "", text)
        return text.strip()

    def truncate(self, text: str, max_chars: int = 8000) -> str:
        if len(text) <= max_chars:
            return text
        # Truncate at last sentence boundary within limit
        truncated = text[:max_chars]
        last_period = truncated.rfind(". ")
        if last_period > max_chars // 2:
            return truncated[: last_period + 1]
        return truncated
