import Darwin
import Foundation

enum CoreClientError: LocalizedError {
    case connection(String)
    case invalidResponse
    case remote(String)

    var errorDescription: String? {
        switch self {
        case .connection(let message): return message
        case .invalidResponse: return "The local search service returned an invalid response."
        case .remote(let message): return message
        }
    }
}

actor CoreClient {
    private let socketPath: String

    init(socketPath: String) { self.socketPath = socketPath }

    func request<Result: Decodable>(
        _ method: String,
        params: [String: Any] = [:],
        as type: Result.Type = Result.self
    ) async throws -> Result {
        let requestID = UUID().uuidString
        let request: [String: Any] = [
            "version": 1,
            "id": requestID,
            "method": method,
            "params": params,
        ]
        let data = try JSONSerialization.data(withJSONObject: request)
        let responseData = try Self.send(data + Data([0x0A]), to: socketPath)
        return try Self.decodeResponse(responseData, requestID: requestID)
    }

    static func decodeResponse<Result: Decodable>(_ data: Data, requestID: String) throws -> Result {
        let envelope = try JSONDecoder.personalSearch.decode(ResponseEnvelope<Result>.self, from: data)
        guard envelope.version == 1, envelope.id == requestID else { throw CoreClientError.invalidResponse }
        if let result = envelope.result, envelope.ok { return result }
        throw CoreClientError.remote(envelope.error?.message ?? "The local search service failed.")
    }

    private static func send(_ request: Data, to path: String) throws -> Data {
        let descriptor = Darwin.socket(AF_UNIX, SOCK_STREAM, 0)
        guard descriptor >= 0 else { throw CoreClientError.connection("Could not create a local search connection.") }
        defer { Darwin.close(descriptor) }

        var address = sockaddr_un()
        address.sun_family = sa_family_t(AF_UNIX)
        let pathBytes = Array(path.utf8CString)
        guard pathBytes.count <= MemoryLayout.size(ofValue: address.sun_path) else {
            throw CoreClientError.connection("The local search socket path is too long.")
        }
        withUnsafeMutableBytes(of: &address.sun_path) { buffer in
            for (index, byte) in pathBytes.enumerated() { buffer[index] = UInt8(bitPattern: byte) }
        }
        let addressLength = socklen_t(MemoryLayout<sa_family_t>.size + pathBytes.count)
        let connected = withUnsafePointer(to: &address) { pointer in
            pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.connect(descriptor, $0, addressLength)
            }
        }
        guard connected == 0 else { throw CoreClientError.connection("The local search service is not ready.") }

        try request.withUnsafeBytes { rawBuffer in
            guard let base = rawBuffer.baseAddress else { return }
            var written = 0
            while written < rawBuffer.count {
                let count = Darwin.write(descriptor, base.advanced(by: written), rawBuffer.count - written)
                guard count > 0 else { throw CoreClientError.connection("Could not send a local search request.") }
                written += count
            }
        }

        var response = Data()
        var buffer = [UInt8](repeating: 0, count: 16_384)
        while true {
            let count = Darwin.read(descriptor, &buffer, buffer.count)
            guard count > 0 else { throw CoreClientError.invalidResponse }
            response.append(contentsOf: buffer.prefix(count))
            if response.last == 0x0A { return response.dropLast() }
            if response.count > 10 * 1_024 * 1_024 { throw CoreClientError.invalidResponse }
        }
    }
}

private struct ResponseEnvelope<Result: Decodable>: Decodable {
    let version: Int
    let id: String
    let ok: Bool
    let result: Result?
    let error: RemoteError?
}

private struct RemoteError: Decodable {
    let code: String
    let message: String
}

private extension JSONDecoder {
    static var personalSearch: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return decoder
    }
}
