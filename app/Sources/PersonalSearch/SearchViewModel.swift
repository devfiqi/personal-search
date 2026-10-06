import AppKit
import Foundation

@MainActor
final class SearchViewModel: ObservableObject {
    @Published var query = ""
    @Published var results: [SearchResult] = []
    @Published var folders: [IndexedFolder] = []
    @Published var gmailAccounts: [GmailAccount] = []
    @Published var indexingState = IndexingState()
    @Published var failures: [ExtractionFailure] = []
    @Published var failureCount = 0
    @Published var connected = false
    @Published var searching = false
    @Published var startupError: String?
    @Published var notice: String?
    @Published var connectingGmail = false

    private var client: CoreClient?
    private var searchTask: Task<Void, Never>?
    private var pollingTask: Task<Void, Never>?
    private var started = false

    func start() async {
        guard AppInstance.isPrimary else { return }
        if started && connected {
            startupError = nil
            await refreshState()
            return
        }
        started = true
        startupError = nil
        do {
            try CoreProcessController.shared.start()
            let client = CoreClient(socketPath: CoreProcessController.shared.socketPath)
            self.client = client
            try await waitUntilReady(client)
            connected = true
            await refreshState()
            startPolling()
        } catch {
            connected = false
            startupError = error.localizedDescription
        }
    }

    func scheduleSearch() {
        searchTask?.cancel()
        let current = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !current.isEmpty else {
            searching = false
            results = []
            return
        }
        searching = true
        searchTask = Task {
            try? await Task.sleep(for: .milliseconds(180))
            guard !Task.isCancelled else { return }
            await search(current)
        }
    }

    func chooseFolder() async {
        let panel = NSOpenPanel()
        panel.title = "Choose a folder to index"
        panel.prompt = "Index Folder"
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.canCreateDirectories = false
        guard panel.runModal() == .OK, let url = panel.url else { return }
        await addFolder(url.path)
    }

    func addFolder(_ path: String) async {
        guard let client else { return }
        do {
            let _: AddFolderResult = try await client.request("add_folder", params: ["path": path])
            notice = nil
            await refreshState()
        } catch { notice = error.localizedDescription }
    }

    func removeFolder(_ folder: IndexedFolder) async {
        guard let client else { return }
        do {
            let _: RemoveFolderResult = try await client.request("remove_folder", params: ["path": folder.path])
            results = []
            query = ""
            notice = nil
            await refreshState()
        } catch { notice = error.localizedDescription }
    }

    func setPaused(_ paused: Bool) async {
        guard let client else { return }
        do {
            indexingState = try await client.request(paused ? "pause" : "resume")
            notice = nil
        } catch { notice = error.localizedDescription }
    }

    func reset() async {
        guard let client else { return }
        do {
            for account in gmailAccounts {
                try GmailCredentials.remove(for: account.email)
            }
            let _: ResetResult = try await client.request("reset")
            query = ""
            results = []
            failures = []
            gmailAccounts = []
            notice = nil
            await refreshState()
        } catch { notice = error.localizedDescription }
    }

    func loadFailures() async {
        guard let client else { return }
        do { failures = try await client.request("failures") }
        catch { notice = error.localizedDescription }
    }

    func connectGmail(clientID: String) async {
        guard let client else { return }
        connectingGmail = true
        defer { connectingGmail = false }
        do {
            let authorization: GmailAuthorization = try await client.request("gmail_connect", params: ["client_id": clientID])
            try GmailCredentials.save(refreshToken: authorization.refreshToken, for: authorization.email)
            do {
                let _: GmailAccount = try await client.request("gmail_add_account", params: [
                    "email": authorization.email, "client_id": authorization.clientID,
                ])
            } catch {
                try? GmailCredentials.remove(for: authorization.email)
                throw error
            }
            notice = nil
            await refreshGmailAccounts()
        } catch {
            notice = error.localizedDescription
        }
    }

    func removeGmail(_ account: GmailAccount) async {
        guard let client else { return }
        do {
            try GmailCredentials.remove(for: account.email)
            let _: RemoveFolderResult = try await client.request("gmail_remove_account", params: ["email": account.email])
            notice = nil
            await refreshGmailAccounts()
        } catch {
            notice = error.localizedDescription
        }
    }

    func openTopResult() {
        guard let result = results.first else { return }
        open(result)
    }

    func open(_ result: SearchResult) {
        NSWorkspace.shared.open(URL(fileURLWithPath: result.path))
    }

    private func search(_ text: String) async {
        guard let client else { return }
        do {
            let found: [SearchResult] = try await client.request("search", params: ["query": text, "limit": 50])
            guard text == query.trimmingCharacters(in: .whitespacesAndNewlines) else { return }
            results = found
            notice = nil
        } catch {
            guard !Task.isCancelled else { return }
            notice = error.localizedDescription
        }
        searching = false
    }

    private func refreshState() async {
        guard let client else { return }
        do {
            let state: ApplicationState = try await client.request("state")
            indexingState = state.indexing
            folders = state.folders
            failureCount = state.failureCount
            await refreshGmailAccounts()
            connected = true
            startupError = nil
        } catch {
            connected = false
            startupError = error.localizedDescription
        }
    }

    private func refreshGmailAccounts() async {
        guard let client else { return }
        do { gmailAccounts = try await client.request("gmail_accounts") }
        catch { notice = error.localizedDescription }
    }

    private func waitUntilReady(_ client: CoreClient) async throws {
        var lastError: Error = CoreClientError.connection("The local search service did not start.")
        for _ in 0..<40 {
            do {
                let _: ApplicationState = try await client.request("state")
                return
            } catch {
                lastError = error
                try await Task.sleep(for: .milliseconds(100))
            }
        }
        throw lastError
    }

    private func startPolling() {
        pollingTask?.cancel()
        pollingTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(1))
                guard !Task.isCancelled else { return }
                await self?.refreshState()
            }
        }
    }

    static var preview: SearchViewModel {
        let model = SearchViewModel()
        model.connected = true
        model.folders = [IndexedFolder(id: 1, path: "/Users/example/Documents")]
        model.query = "project notes"
        model.results = [SearchResult(
            id: 1,
            path: "/Users/example/Documents/Planning/project-notes.md",
            name: "project-notes.md",
            extensionName: ".md",
            modifiedAtNS: Int64(Date().timeIntervalSince1970 * 1_000_000_000),
            snippet: "The [project] launch [notes] and next steps…",
            score: -1,
            matchType: "keyword"
        )]
        return model
    }
}
