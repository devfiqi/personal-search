import Foundation

struct IndexedFolder: Codable, Identifiable, Hashable {
    let id: Int64
    let path: String

    var displayName: String { URL(fileURLWithPath: path).lastPathComponent }
}

struct GmailAccount: Codable, Identifiable, Hashable {
    let email: String
    let clientID: String

    var id: String { email }

    enum CodingKeys: String, CodingKey {
        case email
        case clientID = "client_id"
    }
}

struct GmailAuthorization: Codable {
    let email: String
    let clientID: String
    let refreshToken: String

    enum CodingKeys: String, CodingKey {
        case email
        case clientID = "client_id"
        case refreshToken = "refresh_token"
    }
}

struct SearchResult: Codable, Identifiable, Hashable {
    let id: Int64
    let path: String
    let name: String
    let extensionName: String
    let modifiedAtNS: Int64
    let snippet: String
    let score: Double
    let matchType: String

    enum CodingKeys: String, CodingKey {
        case id, path, name, snippet, score
        case matchType = "match_type"
        case extensionName = "extension"
        case modifiedAtNS = "modified_at_ns"
    }

    var typeLabel: String {
        let value = extensionName.trimmingCharacters(in: CharacterSet(charactersIn: "."))
        return value.isEmpty ? "FILE" : value.uppercased()
    }

    var iconName: String { extensionName == ".pdf" ? "doc.richtext" : "doc.text" }

    var matchLabel: String {
        switch matchType {
        case "semantic": return "RELATED"
        case "hybrid": return "TEXT + RELATED"
        default: return "TEXT MATCH"
        }
    }

    var modifiedLabel: String {
        guard modifiedAtNS > 0 else { return "Unknown date" }
        let date = Date(timeIntervalSince1970: Double(modifiedAtNS) / 1_000_000_000)
        return date.formatted(date: .abbreviated, time: .omitted)
    }
}

struct IndexingState: Codable, Equatable {
    var paused = false
    var indexing = false
    var lastIndexed: String?
    var lastError: String?
    var watchError: String?
    var pendingScan = false
    var watchedPaths = 0

    enum CodingKeys: String, CodingKey {
        case paused, indexing
        case lastIndexed = "last_indexed"
        case lastError = "last_error"
        case watchError = "watch_error"
        case pendingScan = "pending_scan"
        case watchedPaths = "watched_paths"
    }
}

struct ApplicationState: Codable {
    let indexing: IndexingState
    let folders: [IndexedFolder]
    let failureCount: Int

    enum CodingKeys: String, CodingKey {
        case indexing, folders
        case failureCount = "failure_count"
    }
}

struct ExtractionFailure: Codable, Identifiable {
    let id: Int64
    let path: String
    let name: String
    let extensionName: String
    let message: String

    enum CodingKeys: String, CodingKey {
        case id, path, name, message
        case extensionName = "extension"
    }
}

struct RemoveFolderResult: Codable { let removed: Bool }
struct ResetResult: Codable { let status: String }
struct AddFolderResult: Codable { let path: String }
