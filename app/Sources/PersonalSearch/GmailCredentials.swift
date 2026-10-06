import Foundation
import Security

enum GmailCredentials {
    private static let service = "com.devfiqi.personalsearch.gmail"

    static func save(refreshToken: String, for email: String) throws {
        let account = Data(email.utf8)
        let secret = Data(refreshToken.utf8)
        let query: [CFString: Any] = [
            kSecClass: kSecClassGenericPassword,
            kSecAttrService: service,
            kSecAttrAccount: account,
        ]
        let attributes: [CFString: Any] = [kSecValueData: secret]
        let status = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
        if status == errSecItemNotFound {
            var add = query
            add[kSecValueData] = secret
            let addStatus = SecItemAdd(add as CFDictionary, nil)
            guard addStatus == errSecSuccess else { throw CredentialError(status: addStatus) }
        } else if status != errSecSuccess {
            throw CredentialError(status: status)
        }
    }

    static func remove(for email: String) throws {
        let query: [CFString: Any] = [
            kSecClass: kSecClassGenericPassword,
            kSecAttrService: service,
            kSecAttrAccount: Data(email.utf8),
        ]
        let status = SecItemDelete(query as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else { throw CredentialError(status: status) }
    }

    static func refreshToken(for email: String) throws -> String {
        let query: [CFString: Any] = [
            kSecClass: kSecClassGenericPassword,
            kSecAttrService: service,
            kSecAttrAccount: Data(email.utf8),
            kSecReturnData: true,
            kSecMatchLimit: kSecMatchLimitOne,
        ]
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        guard status == errSecSuccess, let data = result as? Data, let token = String(data: data, encoding: .utf8) else {
            throw CredentialError(status: status)
        }
        return token
    }

    private struct CredentialError: LocalizedError {
        let status: OSStatus
        var errorDescription: String? { "Gmail credentials could not be updated in Keychain (\(status))." }
    }
}
