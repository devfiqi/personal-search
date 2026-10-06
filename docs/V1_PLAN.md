# Version 1 Build Plan

## Goal

Build a signed, fully local macOS application that indexes supported documents from user-selected folders and returns keyword-search results within 10 seconds.

The implementation is complete only when the full path works:

```text
Select folder → index documents → search contents → open result
```

## Repository Direction

The existing distributed-service placeholders do not represent the approved first-release architecture.

Implementation should use three clear areas:

- A SwiftUI macOS application
- A Go search and indexing core
- A Python document-extraction worker

The old gateway, coordinator, search-node, and distributed-worker placeholders should be removed when the application scaffold is introduced.

## Milestone 1: Local Search Core

Build the Go foundation:

- Create and migrate the SQLite database.
- Store approved folders and document metadata.
- Create the FTS5 search index.
- Index plain text, Markdown, and supported source-code files.
- Provide keyword querying, ranking, and result snippets.
- Keep original documents outside the database.

Completion gate:

- A command-line test can index a folder and return matching documents from SQLite.
- Reindexing an unchanged folder does not duplicate records.
- Deleted documents disappear from results.
- Search queries are handled safely.

## Milestone 2: PDF Extraction

Build the managed Python extraction worker:

- Bundle a pinned Python runtime and extraction dependencies.
- Extract text from text-based PDFs.
- Return structured success or failure results to Go.
- Keep workers isolated from SQLite.
- Recover from worker crashes and malformed PDFs.

Completion gate:

- Text-based PDFs appear in keyword results.
- Image-only, encrypted, damaged, and unreadable PDFs fail without stopping other indexing work.
- Extracted text remains available for rebuilding the FTS5 index.

## Milestone 3: Incremental Indexing

Keep the index current:

- Watch selected folders while the application runs.
- Process created, changed, moved, and deleted documents.
- Reconcile the selected folders when the application launches.
- Bound background work so search remains responsive.
- Support pausing and resuming indexing.

Completion gate:

- Filesystem changes appear without a complete rescan.
- Changes made while the app was closed are recovered after launch.
- Interrupted work resumes without corrupting or duplicating records.

## Milestone 4: Native macOS Interface

Build the SwiftUI application:

- Present a minimal floating search panel.
- Provide Dock and configurable global-shortcut activation.
- Show an inline folder-selection action when no folders are configured.
- Search as the user types.
- Display file name, type, path, modified date, and matching snippet.
- Open the selected document in its default application.
- Show subtle indexing, paused, and failure states.

Completion gate:

- The complete experience works without a terminal.
- Return opens the top result.
- Clicking a result opens that document.
- Escape hides the search panel.
- Closing the panel does not stop active indexing.

## Milestone 5: Privacy and Recovery

Complete local-data controls:

- Remove derived data when an indexed folder is removed.
- Provide a full local-index reset.
- Prevent document contents and queries from entering logs.
- Retry recoverable extraction failures with a bounded limit.
- Shut down Go, Python, and SQLite cleanly when the app quits.
- Confirm that no component opens a TCP port or contacts a remote service.

Completion gate:

- Reset removes all derived data without modifying original documents.
- Worker and application restarts do not corrupt the index.
- Privacy behavior matches `PRIVACY.md`.

## Milestone 6: Packaging

Prepare the application for direct distribution:

- Bundle the Go executable.
- Bundle the pinned Python runtime and worker dependencies.
- Store runtime data in the correct macOS application-support location.
- Sign and notarize the application.
- Test installation on a clean macOS account.
- Verify that the application works without system Go or Python installations.

Completion gate:

- The signed application launches without development tools.
- Folder selection, indexing, search, and document opening work offline.
- macOS accepts the notarized application without bypassing security controls.

## Testing Strategy

Use:

- Go unit tests for query parsing, indexing state, and ranking
- Python tests for PDF extraction and error handling
- Integration tests for Go, Python, SQLite, and FTS5
- Swift tests for interface state and local API handling
- End-to-end tests for the complete user workflow
- A representative local performance corpus

Required scenarios include:

- Empty folders
- Large folder trees
- Duplicate file names
- Unicode names and content
- Moved and deleted documents
- Permission changes
- Worker crashes
- Interrupted indexing
- Database migrations
- Queries during active indexing

## Version 1 Acceptance

Version 1 is ready when:

- The complete workflow succeeds on a clean supported Mac.
- Indexed queries return results within 10 seconds.
- Search stays responsive during background indexing.
- No personal data or queries leave the computer.
- Failures are visible and recoverable.
- Original documents are never modified.
- All behavior in `PRODUCT.md`, `ARCHITECTURE.md`, and `PRIVACY.md` is satisfied.
