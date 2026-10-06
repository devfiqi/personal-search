GO_TAGS := sqlite_fts5
PYTHON ?= python3

.PHONY: build test setup-extractor build-app build-core test-core test-extractor

build: build-core build-app

test: test-core test-extractor

setup-extractor:
	python3 -m venv extractor/.venv
	extractor/.venv/bin/python -m pip install -e extractor

build-app:
	swift build --package-path app

build-core:
	cd core && go build -tags $(GO_TAGS) -o bin/personal-search-core ./cmd/personal-search-core

test-core:
	cd core && go test -tags $(GO_TAGS) ./...

test-extractor:
	PYTHONPATH=extractor/src $(PYTHON) -m unittest discover -s extractor/tests -v
