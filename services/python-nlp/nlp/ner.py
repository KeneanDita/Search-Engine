"""Named Entity Recognition using spaCy."""
from __future__ import annotations

from typing import Any

import structlog

logger = structlog.get_logger(__name__)

ENTITY_TYPES = {
    "PERSON", "ORG", "GPE", "LOC", "PRODUCT", "EVENT",
    "WORK_OF_ART", "LAW", "LANGUAGE", "DATE", "MONEY", "NORP",
}


class NERExtractor:
    def __init__(self) -> None:
        from nlp.tokenizer import _load_spacy
        self._nlp = _load_spacy()

    def extract(self, text: str) -> dict[str, list[str]]:
        """Return dict of entity_type → [entity_text, ...]."""
        if not self._nlp:
            return {}
        doc = self._nlp(text[:50_000])
        entities: dict[str, list[str]] = {}
        seen: set[tuple[str, str]] = set()
        for ent in doc.ents:
            if ent.label_ not in ENTITY_TYPES:
                continue
            key = (ent.label_, ent.text.strip().lower())
            if key in seen:
                continue
            seen.add(key)
            entities.setdefault(ent.label_, []).append(ent.text.strip())
        return entities

    def extract_flat(self, text: str) -> list[dict[str, str]]:
        """Return list of {text, label} dicts."""
        entities = self.extract(text)
        return [
            {"text": ent, "label": label}
            for label, ents in entities.items()
            for ent in ents
        ]
