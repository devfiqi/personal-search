import Darwin
import Foundation

@MainActor
final class CoreProcessController {
    static let shared = CoreProcessController()

    var socketPath: String {
        if let override = ProcessInfo.processInfo.environment["PERSONAL_SEARCH_SOCKET"], !override.isEmpty {
            return override
        }
        return "/tmp/personal-search-\(getuid()).sock"
    }
    private var process: Process?

    private init() {}

    static func supportDirectory() throws -> URL {
        let fileManager = FileManager.default
        let support: URL
        if let override = ProcessInfo.processInfo.environment["PERSONAL_SEARCH_SUPPORT"], !override.isEmpty {
            support = URL(fileURLWithPath: override, isDirectory: true)
        } else {
            support = try fileManager.url(
                for: .applicationSupportDirectory,
                in: .userDomainMask,
                appropriateFor: nil,
                create: true
            ).appendingPathComponent("PersonalSearch", isDirectory: true)
        }
        try fileManager.createDirectory(at: support, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        return support
    }

    func start() throws {
        if process?.isRunning == true { return }
        let support = try Self.supportDirectory()

        let task = Process()
        task.executableURL = try locateCore()
        task.arguments = [
            "serve",
            "--db", support.appendingPathComponent("search.db").path,
            "--socket", socketPath,
        ] + extractorArguments()
        var environment = ProcessInfo.processInfo.environment
        if let resources = Bundle.main.resourceURL {
            let modelCache = resources.appendingPathComponent("Models/FastEmbed")
            if FileManager.default.fileExists(atPath: modelCache.path) {
                environment["PERSONAL_SEARCH_EMBEDDING_MODEL_DIR"] = modelCache.path
                environment["HF_HUB_OFFLINE"] = "1"
            }
        }
        environment["PERSONAL_SEARCH_SEMANTIC_INDEX_PATH"] = support.appendingPathComponent("semantic.hnsw").path
        task.environment = environment
        task.standardOutput = FileHandle.nullDevice
        task.standardError = FileHandle.nullDevice
        try task.run()
        process = task
    }

    func stop() {
        guard let process else { return }
        if process.isRunning {
            process.terminate()
            process.waitUntilExit()
        }
        self.process = nil
    }

    private func locateCore() throws -> URL {
        let environment = ProcessInfo.processInfo.environment
        let fileManager = FileManager.default
        var candidates: [URL] = []
        if let configured = environment["PERSONAL_SEARCH_CORE"] { candidates.append(URL(fileURLWithPath: configured)) }
        if let bundled = Bundle.main.url(forAuxiliaryExecutable: "personal-search-core") { candidates.append(bundled) }
        if let resources = Bundle.main.resourceURL { candidates.append(resources.appendingPathComponent("Core/personal-search-core")) }
        let current = URL(fileURLWithPath: fileManager.currentDirectoryPath)
        candidates.append(current.appendingPathComponent("core/bin/personal-search-core"))
        candidates.append(current.appendingPathComponent("../core/bin/personal-search-core").standardizedFileURL)
        if let match = candidates.first(where: { fileManager.isExecutableFile(atPath: $0.path) }) { return match }
        throw CoreClientError.connection("The bundled local search service could not be found.")
    }

    private func extractorArguments() -> [String] {
        let environment = ProcessInfo.processInfo.environment
        let fileManager = FileManager.default
        let current = URL(fileURLWithPath: fileManager.currentDirectoryPath)
        var arguments: [String] = []
        if let resources = Bundle.main.resourceURL {
            let bundledWorker = resources.appendingPathComponent("Extractor/personal-search-extractor")
            if fileManager.isExecutableFile(atPath: bundledWorker.path) {
                return ["--extractor-bin", bundledWorker.path]
            }
        }
        let pythonCandidates = [
            environment["PERSONAL_SEARCH_PYTHON"].map(URL.init(fileURLWithPath:)),
            current.appendingPathComponent("extractor/.venv/bin/python"),
        ].compactMap { $0 }
        if let python = pythonCandidates.first(where: { fileManager.isExecutableFile(atPath: $0.path) }) {
            arguments += ["--python", python.path]
        }
        let sourceCandidates = [
            environment["PERSONAL_SEARCH_EXTRACTOR_SRC"].map(URL.init(fileURLWithPath:)),
            current.appendingPathComponent("extractor/src"),
        ].compactMap { $0 }
        if let source = sourceCandidates.first(where: { fileManager.fileExists(atPath: $0.path) }) {
            arguments += ["--extractor-src", source.path]
        }
        return arguments
    }
}
