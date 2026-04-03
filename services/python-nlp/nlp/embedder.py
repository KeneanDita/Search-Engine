"""Text embedding using Sentence-Transformers (free, local, no API key)."""
from __future__ import annotations

import functools
import os
from typing import Any

import numpy as np
import structlog

logger = structlog.get_logger(__name__)

MODEL_NAME = os.getenv("EMBEDDING_MODEL", "all-MiniLM-L6-v2")
# all-MiniLM-L6-v2: 384-dim, ~80MB, very fast, excellent quality


@functools.lru_cache(maxsize=1)
def _load_model(model_name: str) -> Any:
    from sentence_transformers import SentenceTransformer
    logger.info("loading_embedding_model", model=model_name)
    model = SentenceTransformer(model_name)
    logger.info("embedding_model_loaded", model=model_name)
    return model


class Embedder:
    def __init__(self, model_name: str = MODEL_NAME) -> None:
        self._model_name = model_name
        self._model: Any = None

    def _get_model(self) -> Any:
        if self._model is None:
            self._model = _load_model(self._model_name)
        return self._model

    def embed(self, text: str) -> list[float]:
        """Return a normalised embedding vector for a single text."""
        model = self._get_model()
        vec: np.ndarray = model.encode(text, normalize_embeddings=True, show_progress_bar=False)
        return vec.tolist()

    def embed_batch(self, texts: list[str], batch_size: int = 64) -> list[list[float]]:
        """Return embeddings for a batch of texts."""
        if not texts:
            return []
        model = self._get_model()
        vecs: np.ndarray = model.encode(
            texts,
            batch_size=batch_size,
            normalize_embeddings=True,
            show_progress_bar=False,
        )
        return vecs.tolist()

    def cosine_similarity(self, a: list[float], b: list[float]) -> float:
        """Cosine similarity between two pre-normalised vectors."""
        va = np.array(a, dtype=np.float32)
        vb = np.array(b, dtype=np.float32)
        return float(np.dot(va, vb))

    @property
    def dimension(self) -> int:
        model = self._get_model()
        return int(model.get_sentence_embedding_dimension())
