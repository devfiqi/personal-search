# AGENTS.md

## Project

Personal Search is a fully local macOS search engine for personal documents and, in later phases, other personal media.

Privacy is a core requirement:

- User content, extracted text, indexes, and queries remain on-device.
- Do not add cloud processing, telemetry, remote storage, or external search services.
- Any feature requiring network access needs explicit owner approval.

## Approved Direction

The current architecture is:

- SwiftUI for the native macOS interface
- Go for the search backend, indexing, and orchestration
- Python for document extraction and future machine-learning workloads
- SQLite with FTS5 for storage and initial keyword search

The first usable release is limited to:

- User-selected folders
- Plain text, Markdown, source code, and text-based PDFs
- Keyword search
- A minimal floating search interface
- Opening selected results in their default macOS application
- Background indexing while the application is running

Photos, video, email, OCR, semantic search, and vector indexing are later phases.

## Decision Ownership

The project owner has final approval over architecture and product design.

- Do not introduce a new language, database, service, major framework, or architectural boundary without approval.
- Present material decisions with concise tradeoffs and a recommendation.
- Avoid interrupting the owner for low-level, reversible implementation details.
- Do not treat proposed ideas as approved decisions.

## Documentation Approval

Every Markdown file requires explicit owner approval.

- Draft and review one Markdown file at a time.
- Do not create, edit, rename, move, or delete a Markdown file before its specific contents are approved.
- After approval, apply only the approved document change.
- Keep public documentation free of private paths, credentials, and machine-specific information.

`AGENTS.override.md` contains private local instructions and must never be committed.

## Engineering Practices

- Inspect existing code and repository state before making changes.
- Keep changes focused on the approved task.
- Preserve unrelated user changes.
- Prefer simple, replaceable components over premature distribution.
- Keep expensive extraction work separate from interactive search.
- Add tests appropriate to the behavior being introduced.
- Verify changes before reporting completion.
