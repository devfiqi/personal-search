#!/bin/zsh

set -euo pipefail

repository_root=${0:A:h:h}
cache_directory="$repository_root/tmp/embedding-model"
python="$repository_root/extractor/.venv/bin/python"

if [[ ! -x "$python" ]]; then
    print -u2 "Run 'make setup-extractor' before preparing the semantic model"
    exit 1
fi

if [[ -d "$cache_directory/models--qdrant--paraphrase-multilingual-MiniLM-L12-v2-onnx-Q" ]]; then
    print "$cache_directory"
    exit 0
fi

mkdir -p "$cache_directory"
"$python" -c '
from fastembed import TextEmbedding
TextEmbedding("sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2", cache_dir="'"$cache_directory"'")
'
print "$cache_directory"
