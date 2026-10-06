# Architecture

## Overview

Personal Search is a single-machine macOS application composed of three local layers:

- A SwiftUI interface
- A Go search and indexing core
- Managed Python extraction workers

SQLite is the source of truth and provides initial full-text search through FTS5.

The first release is intentionally a local modular application, not a collection of distributed services. Components may be separated further in later phases when there is a demonstrated need.

## Principles

- All personal data and processing remain on-device.
- Interactive search stays separate from expensive extraction work.
- Original documents remain in their existing locations.
- Extracted content is retained so search indexes can be rebuilt without reprocessing every document.
- Failures processing one document must not stop indexing other documents.
- Components should remain replaceable as requirements become clearer.
- The first release should favor reliability and simplicity over premature distribution.

## Runtime Components

### SwiftUI Application

The native macOS application owns:

- The floating search interface
- Folder selection
- Search result presentation
- Global-shortcut and Dock activation
- Opening documents in their default applications
- Starting and monitoring the Go process
- Displaying indexing state and extraction failures

The interface does not read from SQLite directly.

### Go Core

The Go process owns:

- Approved-folder registration
- Initial directory scanning
- Filesystem change detection
- Launch-time reconciliation
- Indexing coordination
- Plain-text and source-code extraction
- SQLite access and migrations
- FTS5 indexing and querying
- Search ranking and result snippets
- Python-worker supervision
- The private local API used by SwiftUI

The Go process remains responsive to searches while indexing continues in the background.

### Python Workers

Python workers initially handle text extraction from text-based PDFs.

Workers:

- Are launched and supervised by Go
- Receive individual extraction jobs
- Return structured extraction results
- Do not write directly to SQLite
- Do not expose a network service
- Can fail or restart without taking down search

The application bundles its own pinned Python runtime and dependencies. It does not depend on the user’s system Python or download packages at runtime.

### SQLite

SQLite stores:

- Approved folder records
- Document metadata
- Extracted text
- Processing state
- Extraction errors
- Schema versions
- The FTS5 keyword index

SQLite does not store copies of original documents.

The database and derived application data live in the application’s private macOS support directory.

## Communication

SwiftUI communicates with Go through a private Unix-domain socket using versioned JSON messages.

Go communicates with managed Python workers through local structured messages.

No component opens a TCP port or requires network access.

## Indexing Flow

1. The user selects one or more folders.
2. SwiftUI passes the approved folder paths to Go.
3. Go scans supported files and compares their metadata with SQLite.
4. Go extracts supported text files directly.
5. Go sends text-based PDFs to a Python worker.
6. Extraction results are written to SQLite in transactions.
7. FTS5 is updated with searchable text.
8. Filesystem events trigger incremental updates while the application runs.
9. A lightweight reconciliation scan at launch recovers changes made while the application was closed.

Deleted and moved files are removed or updated without requiring a complete index rebuild.

## Search Flow

1. SwiftUI sends a debounced query to Go.
2. Go converts the query into a safe FTS5 query.
3. SQLite returns ranked matches and highlighted snippets.
4. Go returns result metadata to SwiftUI.
5. SwiftUI displays the results.
6. Activating a result opens the original file through macOS.

Search requests take priority over background indexing work.

## Lifecycle

- Opening the application starts the Go process.
- Go starts Python workers only when extraction work requires them.
- Closing the search window does not stop indexing.
- Explicitly quitting the application shuts down workers and closes SQLite cleanly.
- Incomplete jobs remain recoverable on the next launch.
- The first release does not install an always-running login helper.

## Failure Handling

- Unsupported or unreadable files are recorded and skipped.
- Encrypted, damaged, or image-only PDFs produce a visible extraction failure.
- Python-worker crashes are isolated and may be retried with a bounded retry count.
- Interrupted indexing resumes after the next launch.
- Database migrations run before indexing or search begins.
- A damaged derived search index can be rebuilt from stored metadata and extracted content.

## Security and Privacy Boundaries

- No document content leaves the computer.
- No telemetry or analytics are collected.
- No cloud APIs or remote models are used.
- Only explicitly selected folders are indexed.
- The local API is not exposed over the network.
- Logs must not contain full document contents or search queries.
- Future network access requires a separately approved architecture decision.

## Deferred Architecture

The first release does not introduce:

- Distributed search nodes
- A coordinator service
- A separate vector database
- Bleve
- `sqlite-vec`
- Semantic or image embeddings
- Cloud synchronization
- Cross-platform abstractions

These may be evaluated in later phases without changing SQLite’s role as the canonical metadata and extracted-content store.
