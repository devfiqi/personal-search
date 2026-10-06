# Personal Search

A fully local macOS application for finding personal documents by searching their contents.

> Search for what you remember, not where you stored it.

## Overview

Personal Search indexes documents from folders you explicitly select and makes their contents searchable through a minimal native interface.

All documents, extracted text, indexes, and queries remain on-device. The application does not use cloud processing, remote storage, telemetry, or external search services.

## Version 1

The first release will support:

- Plain-text files
- Markdown files
- Source-code files
- Text-based PDFs
- Keyword search
- User-selected folders
- Background indexing while the application runs
- A floating native macOS search panel
- Opening results in their default applications

Photos, screenshots, video, email, OCR, and semantic search are planned as possible later phases and are not part of the first release.

## Experience

```text
Select folders → index documents → search contents → open a result
```

Search results show the document name, type, path, modified date, and a snippet containing the matching terms.

Queries run against a local index instead of scanning every document at search time.

## Architecture

Personal Search uses:

- SwiftUI for the macOS interface
- Go for indexing, keyword search, and application orchestration
- Python for document extraction and future machine-learning workloads
- SQLite with FTS5 for local storage and full-text search

The first release is a modular application running entirely on one computer. It does not require separate servers or cloud infrastructure.

Future distributed-systems experiments may split work across multiple local processes while preserving the on-device privacy boundary.

## Privacy

Original documents remain in their existing locations and are never copied into the application database.

Removing an indexed folder deletes its derived search data without modifying the original files.

See [Privacy and Data Handling](docs/PRIVACY.md) for the complete policy.

## Documentation

- [Product Requirements](docs/PRODUCT.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Privacy and Data Handling](docs/PRIVACY.md)
- [Version 1 Build Plan](docs/V1_PLAN.md)

## Status

The project is in the architecture and initial implementation phase.
