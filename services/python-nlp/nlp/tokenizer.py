"""Tokenization and stopword removal using spaCy + NLTK."""
from __future__ import annotations

import functools
from typing import Any

import nltk
import spacy
import structlog

logger = structlog.get_logger(__name__)

# Download NLTK data on first use
nltk.download("stopwords", quiet=True)
nltk.download("punkt", quiet=True)

from nltk.corpus import stopwords as _nltk_stopwords  # noqa: E402

_NLTK_STOPWORDS: frozenset[str] = frozenset(_nltk_stopwords.words("english"))


@functools.lru_cache(maxsize=1)
def _load_spacy() -> Any:
    try:
        return spacy.load("en_core_web_sm")
    except OSError:
        logger.warning("spacy_model_missing", model="en_core_web_sm", hint="run: python -m spacy download en_core_web_sm")
        return None


class Tokenizer:
    def __init__(self) -> None:
        self._nlp = _load_spacy()

    def tokenize(self, text: str) -> list[str]:
        """Return list of lowercase alphabetic tokens, stopwords removed."""
        if self._nlp:
            doc = self._nlp(text[:100_000])
            return [
                token.lemma_.lower()
                for token in doc
                if token.is_alpha and not token.is_stop and len(token) > 1
            ]
        # Fallback: NLTK
        tokens = nltk.word_tokenize(text.lower())
        return [t for t in tokens if t.isalpha() and t not in _NLTK_STOPWORDS and len(t) > 1]

    def sentences(self, text: str) -> list[str]:
        """Split text into sentences."""
        if self._nlp:
            doc = self._nlp(text[:100_000])
            return [sent.text.strip() for sent in doc.sents if sent.text.strip()]
        return nltk.sent_tokenize(text)

    def keyphrases(self, text: str, top_n: int = 20) -> list[str]:
        """Extract noun-chunk keyphrases using spaCy."""
        if not self._nlp:
            return self.tokenize(text)[:top_n]
        doc = self._nlp(text[:50_000])
        phrases = [
            chunk.text.lower().strip()
            for chunk in doc.noun_chunks
            if len(chunk.text) > 2 and not all(t.is_stop for t in chunk)
        ]
        # Deduplicate while preserving order
        seen: set[str] = set()
        result: list[str] = []
        for p in phrases:
            if p not in seen:
                seen.add(p)
                result.append(p)
        return result[:top_n]
