# Privacy and Data Handling

## Privacy Promise

Personal Search is an offline, on-device application.

The application does not upload documents, extracted text, metadata, search queries, or indexes. It does not use cloud processing, telemetry, analytics, advertising, or remote models.

Adding any network-dependent feature requires a separately approved architecture and privacy decision.

## Indexed Content

The application accesses only folders explicitly selected by the user.

Original documents remain in their existing locations and are never copied into the application database.

For supported documents, the application may store:

- File name and path
- File type and size
- Creation and modification timestamps
- Extracted text
- Processing status and errors
- Full-text search data

This derived data exists only to provide local indexing and search.

## Local Storage

The SQLite database, search index, and operational files are stored in the application’s private macOS support directory.

The first release does not add application-level database encryption. Local data is protected by the user’s macOS account, filesystem permissions, and FileVault when enabled.

The application does not intentionally place indexed data in iCloud or another synchronized folder. Operating-system backups, including Time Machine, may include application data according to the user’s backup settings.

## Search Queries

Search queries are processed locally.

Queries are not:

- Sent over a network
- Stored as search history
- Included in analytics
- Written to persistent logs

Queries may exist briefly in application memory while a search is active.

## Logs and Errors

Logs must not contain:

- Full document contents
- Extracted text
- Search queries
- Credentials
- Private environment values

Persistent logs should use internal document identifiers instead of full file paths whenever possible.

The interface may display a file path or extraction error when needed to help the user identify a document.

## Data Removal

When a selected folder is removed from the application:

- Its documents are removed from the searchable index.
- Stored extracted text and related metadata are deleted.
- The original files remain untouched.

When a document is deleted from disk, its derived records are removed during filesystem processing or launch-time reconciliation.

The application must provide a reset action that deletes the entire local index and all derived data without deleting original documents.

## Process Boundaries

- SwiftUI communicates with Go through a private Unix-domain socket.
- Python extraction workers are local child processes.
- No component listens on a TCP network port.
- Python workers receive only the document information required for their current extraction job.
- Temporary extraction data must be deleted after processing or failure.

## Permissions

Folder access begins only after explicit user selection.

The application must:

- Clearly show which folders are indexed.
- Allow folders to be removed.
- Avoid scanning parent or neighboring folders that were not selected.
- Handle denied or revoked access without bypassing macOS protections.
- Never request broader permissions than the current feature requires.

## Threat Model

The first release protects against accidental disclosure by the application and prevents intentional cloud transmission.

It does not claim to protect indexed data from:

- Another process running as the same macOS user
- A system administrator
- Malware with equivalent or greater permissions
- Someone with access to an unlocked computer
- Operating-system backups configured by the user

Users with stronger local-security requirements should enable FileVault and secure their macOS account.

## Future Features

Photos, email, video, OCR, semantic search, embeddings, synchronization, and remote access require updated privacy review before implementation.

Future documentation must explain what new data is read, what derived data is stored, how it is deleted, and whether any new process or network boundary is introduced.
