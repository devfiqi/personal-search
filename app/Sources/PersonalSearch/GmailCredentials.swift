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

    private struct CredentialError: LocalizedError {
        let status: OSStatus
        var errorDescription: String? { "Gmail credentials could not be updated in Keychain (\(status))." }
    }
}
