import os
from functools import lru_cache

from fastembed import TextEmbedding


MODEL_NAME = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"
MAX_TEXTS_PER_REQUEST = 32
MAX_TEXT_BYTES = 24_000


class SemanticEmbeddingError(Exception):
    pass


@lru_cache(maxsize=1)
def model() -> TextEmbedding:
    cache_directory = os.environ.get("PERSONAL_SEARCH_EMBEDDING_MODEL_DIR")
    if not cache_directory:
        raise SemanticEmbeddingError("semantic model is not bundled")
    try:
        return TextEmbedding(
            MODEL_NAME,
            cache_dir=cache_directory,
            local_files_only=True,
        )
    except Exception as error:
        raise SemanticEmbeddingError(f"semantic model is unavailable: {error}") from error


def embed(texts: list[str]) -> list[list[float]]:
    if not texts or len(texts) > MAX_TEXTS_PER_REQUEST:
        raise SemanticEmbeddingError("embed requires between 1 and 32 texts")
    if any(not text or len(text.encode("utf-8")) > MAX_TEXT_BYTES for text in texts):
        raise SemanticEmbeddingError("semantic text is empty or exceeds the size limit")
    return [vector.astype(float).tolist() for vector in model().embed(texts)]
