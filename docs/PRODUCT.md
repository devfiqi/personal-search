# Product Requirements

## Summary

Personal Search is a fully local macOS application for finding personal information by searching its contents instead of remembering where it was stored.

All indexing, storage, and search happen on-device.

## Initial User

The first version is designed for a single macOS user searching documents stored in folders they explicitly select.

## Product Goals

- Make personal documents searchable by their contents.
- Return results from indexed content within 10 seconds.
- Keep all personal data and queries on-device.
- Open a selected result in its default macOS application.
- Keep the index current while the application is running.
- Provide a minimal search experience without a dashboard.

## First Release

The first usable release supports:

- User-selected folders
- Plain-text files
- Markdown files
- Source-code files
- Text-based PDF files
- Keyword search
- Search-as-you-type
- A floating SwiftUI search panel
- Dock and configurable global-shortcut activation
- Background indexing while the application runs
- Opening selected results in their default application

The interface shows a quiet folder-selection action when no folders have been configured.

## Search Results

Each result should provide enough context to identify the document:

- File name
- File type
- File path
- Modified date
- A matching text snippet

Results containing all query terms should rank above partial matches. Quoted text should be treated as an exact phrase.

Activating a result opens the original file. The first release does not include an inline document viewer.

## Out of Scope

The first release does not include:

- Cloud processing or storage
- Telemetry
- Photos or screenshots
- Video
- Email
- OCR or scanned-PDF extraction
- Semantic search
- Embeddings or vector search
- Office or Apple iWork documents
- Windows or Linux support
- Indexing while the application is fully quit

These capabilities may be considered in later phases but are not implied commitments.

## Success Criteria

The first release is successful when:

- A user can select one or more folders.
- Supported documents are indexed without uploading their contents.
- Keyword queries return relevant indexed documents within 10 seconds.
- Search remains responsive while background indexing is active.
- New, changed, moved, and deleted documents are reflected in the index while the app runs.
- Changes made while the app was closed are discovered after it reopens.
- Selecting a result opens the correct original document.
- Indexing can be paused and resumed.
- Extraction failures are reported without stopping the rest of the index.
