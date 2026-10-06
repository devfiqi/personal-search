GO_TAGS := sqlite_fts5

.PHONY: build test build-app build-core test-core test-extractor

build: build-core build-app

test: test-core test-extractor

build-app:
	swift build --package-path app

build-core:
	cd core && go build -tags $(GO_TAGS) -o bin/personal-search-core ./cmd/personal-search-core

test-core:
	cd core && go test -tags $(GO_TAGS) ./...

test-extractor:
	cd extractor && PYTHONPATH=src python3 -m unittest discover -s tests -v
