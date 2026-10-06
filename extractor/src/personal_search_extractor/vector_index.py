import os
from functools import lru_cache
from pathlib import Path

import hnswlib
import numpy as np


DIMENSIONS = 384
INITIAL_CAPACITY = 10_000


class VectorIndexError(Exception):
    pass


class VectorIndex:
    def __init__(self, path: str) -> None:
        self.path = Path(path)
        self.path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        self.index = hnswlib.Index(space="cosine", dim=DIMENSIONS)
        if self.path.exists():
            self.index.load_index(str(self.path))
        else:
            self.index.init_index(
                max_elements=INITIAL_CAPACITY,
                ef_construction=200,
                M=16,
                allow_replace_deleted=True,
            )
        self.index.set_ef(100)

    def upsert(self, ids: list[int], vectors: list[list[float]]) -> None:
        if not ids or len(ids) != len(vectors):
            raise VectorIndexError("vector IDs and values must have the same non-zero length")
        array = np.asarray(vectors, dtype=np.float32)
        if array.ndim != 2 or array.shape[1] != DIMENSIONS:
            raise VectorIndexError(f"vectors must have {DIMENSIONS} dimensions")
        needed = self.index.get_current_count() + len(ids)
        if needed > self.index.get_max_elements():
            self.index.resize_index(max(needed, self.index.get_max_elements() * 2))
        self.index.add_items(array, np.asarray(ids, dtype=np.int64), replace_deleted=True)
        self.index.save_index(str(self.path))

    def search(self, vector: list[float], limit: int) -> list[tuple[int, float]]:
        if limit <= 0:
            return []
        if self.index.get_current_count() == 0:
            return []
        array = np.asarray([vector], dtype=np.float32)
        if array.shape != (1, DIMENSIONS):
            raise VectorIndexError(f"query vector must have {DIMENSIONS} dimensions")
        count = min(limit, self.index.get_current_count())
        ids, distances = self.index.knn_query(array, k=count)
        return [(int(chunk_id), float(distance)) for chunk_id, distance in zip(ids[0], distances[0])]


@lru_cache(maxsize=1)
def local_index() -> VectorIndex:
    path = os.environ.get("PERSONAL_SEARCH_SEMANTIC_INDEX_PATH")
    if not path:
        raise VectorIndexError("semantic index path is not configured")
    return VectorIndex(path)
