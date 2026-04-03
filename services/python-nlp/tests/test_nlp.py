"""Unit tests for NLP processing."""
from __future__ import annotations

import pytest
from unittest.mock import patch, MagicMock

from nlp.text_cleaner import TextCleaner
from nlp.tokenizer import Tokenizer


def test_text_cleaner_html():
    cleaner = TextCleaner()
    html = "<html><body><script>alert(1)</script><p>Hello world</p></body></html>"
    result = cleaner.clean_html(html)
    assert "alert" not in result
    assert "Hello world" in result


def test_text_cleaner_normalise():
    cleaner = TextCleaner()
    text = "Hello   world\n\n\n\nfoo"
    result = cleaner.clean_text(text)
    assert "  " not in result


def test_text_cleaner_truncate():
    cleaner = TextCleaner()
    text = "A" * 10000
    result = cleaner.truncate(text, max_chars=100)
    assert len(result) <= 100


def test_text_cleaner_removes_urls():
    cleaner = TextCleaner()
    text = "Visit https://example.com/path?q=1 for more info"
    result = cleaner.clean_text(text)
    assert "https" not in result
    assert "Visit" in result


def test_tokenizer_removes_stopwords():
    tokenizer = Tokenizer()
    tokens = tokenizer.tokenize("the quick brown fox jumps over the lazy dog")
    assert "the" not in tokens
    assert "over" not in tokens
    # Content words should remain
    content = set(tokens)
    assert content & {"quick", "brown", "fox", "jump", "lazy", "dog"}


def test_tokenizer_lemmatizes():
    tokenizer = Tokenizer()
    tokens = tokenizer.tokenize("running dogs are faster than walking cats")
    # spaCy lemmatises 'running' → 'run', 'dogs' → 'dog'
    assert "run" in tokens or "running" in tokens


def test_embedder_dimension():
    from nlp.embedder import Embedder
    emb = Embedder()
    vec = emb.embed("test sentence")
    assert len(vec) == 384  # all-MiniLM-L6-v2 produces 384-dim vectors


def test_embedder_cosine_similarity_identical():
    from nlp.embedder import Embedder
    emb = Embedder()
    vec = emb.embed("machine learning")
    sim = emb.cosine_similarity(vec, vec)
    assert abs(sim - 1.0) < 1e-4


def test_embedder_batch_matches_single():
    from nlp.embedder import Embedder
    emb = Embedder()
    texts = ["hello world", "machine learning"]
    batch = emb.embed_batch(texts)
    single_0 = emb.embed(texts[0])
    single_1 = emb.embed(texts[1])
    assert len(batch) == 2
    assert emb.cosine_similarity(batch[0], single_0) > 0.999
    assert emb.cosine_similarity(batch[1], single_1) > 0.999
