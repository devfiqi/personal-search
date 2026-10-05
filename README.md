# Personal Search

A local search engine for personal data, built as a practical way to learn and implement distributed systems fundamentals.

## Overview

Personal Search is a native desktop search engine designed to make information across a computer easy to retrieve regardless of where it is stored.

The goal is to search across personal data using both exact keyword matching and semantic meaning, allowing queries such as:

- `notes about replication lag`
- `the document where I mentioned Flink`
- `screenshots containing my offer details`
- `photos from a specific trip`
- `emails about an upcoming event`

The system is intended to support multiple data sources, including:

- Local files
- Documents
- Source code
- PDFs
- Screenshots
- Photos
- Email
- Browser data
- Additional personal data sources

All personal data and indexes are designed to remain on-device.

The long-term goal is to evolve the project into a distributed search system inspired by concepts from *Designing Data-Intensive Applications*.

The final system is expected to support:

- Keyword and semantic search
- Search across text, images, and other personal data
- Automatic indexing as files and data change
- Multiple workers processing data in parallel
- Search data split across multiple nodes
- Replicated copies of search data
- Search across multiple nodes at once
- Recovery when a worker or node fails
- Safe retries when work is interrupted
- Rebuilding indexes without taking search offline
- Local-first privacy

The purpose of the project is not only to build a useful personal search engine, but also to provide a real system for practicing distributed systems concepts such as replication, partitioning, consistency, coordination, and fault tolerance.

## Motivation

Computers accumulate large amounts of useful information across files, screenshots, documents, code, photos, email, downloads, and other sources.

Finding that information often requires remembering where something was saved, what it was called, or which application contained it.

Personal Search approaches the problem differently:

> Search for what you remember, not where you stored it.

Instead of relying mainly on filenames and folders, the system makes personal data searchable by its actual content and meaning.

The project also gives distributed systems concepts a real purpose. As the amount of indexed data grows, the system can evolve to split work across multiple processes, keep copies of important data, recover from failures, and search large indexes efficiently.

## Architecture

```text
personal-search/
├── gateway/
├── indexer/
├── search-node/
├── coordinator/
├── worker/
├── ui/
├── shared/
└── docs/
```

- `gateway` — entry point between the desktop app and the search system
- `indexer` — discovers data and prepares it for search
- `search-node` — stores and searches indexed data
- `coordinator` — manages work across multiple search nodes
- `worker` — handles background processing
- `ui` — native desktop application
- `shared` — common types and utilities
- `docs` — architecture notes and project documentation
