import AppKit
import Carbon
import SwiftUI

enum SearchShortcut: String, CaseIterable, Identifiable {
    case commandShiftSpace
    case optionSpace
    case controlSpace

    var id: String { rawValue }

    var label: String {
        switch self {
        case .commandShiftSpace: return "Command–Shift–Space"
        case .optionSpace: return "Option–Space"
        case .controlSpace: return "Control–Space"
        }
    }

    var modifiers: UInt32 {
        switch self {
        case .commandShiftSpace: return UInt32(cmdKey | shiftKey)
        case .optionSpace: return UInt32(optionKey)
        case .controlSpace: return UInt32(controlKey)
        }
    }
}

@MainActor
final class HotKeyManager: ObservableObject {
    static let shared = HotKeyManager()
    @Published private(set) var registrationFailed = false
    private var hotKey: EventHotKeyRef?
    private var handler: EventHandlerRef?
    private var current: SearchShortcut?

    var activeShortcut: SearchShortcut? { current }

    private init() {
        var eventType = EventTypeSpec(eventClass: OSType(kEventClassKeyboard), eventKind: UInt32(kEventHotKeyPressed))
        InstallEventHandler(
            GetApplicationEventTarget(),
            { _, _, _ in
                Task { @MainActor in
                    SearchWindow.show()
                }
                return noErr
            },
            1,
            &eventType,
            nil,
            &handler
        )
    }

    func registerSavedShortcut() {
        let saved = UserDefaults.standard.string(forKey: "searchShortcut")
        let preferred = SearchShortcut(rawValue: saved ?? "") ?? .commandShiftSpace
        if register(preferred) {
            UserDefaults.standard.set(preferred.rawValue, forKey: "searchShortcut")
            return
        }
        for shortcut in SearchShortcut.allCases where shortcut != preferred && register(shortcut) {
            UserDefaults.standard.set(shortcut.rawValue, forKey: "searchShortcut")
            return
        }
    }

    @discardableResult
    func register(_ shortcut: SearchShortcut) -> Bool {
        if current == shortcut, hotKey != nil, !registrationFailed {
            return true
        }
        let previous = current
        let previousHotKey = hotKey
        if let previousHotKey {
            UnregisterEventHotKey(previousHotKey)
            hotKey = nil
        }
        if install(shortcut) {
            current = shortcut
            registrationFailed = false
            return true
        }
        if let previous, install(previous) {
            current = previous
        }
        registrationFailed = true
        return false
    }

    private func install(_ shortcut: SearchShortcut) -> Bool {
        var registered: EventHotKeyRef?
        let status = RegisterEventHotKey(
            UInt32(kVK_Space),
            shortcut.modifiers,
            EventHotKeyID(signature: OSType(0x50535243), id: 1),
            GetApplicationEventTarget(),
            0,
            &registered
        )
        guard status == noErr else { return false }
        hotKey = registered
        return true
    }
}

struct ShortcutSettingsView: View {
    @AppStorage("searchShortcut") private var shortcut = SearchShortcut.commandShiftSpace.rawValue
    @ObservedObject private var hotKeys = HotKeyManager.shared

    var body: some View {
        Form {
            Picker("Open Personal Search", selection: $shortcut) {
                ForEach(SearchShortcut.allCases) { option in
                    Text(option.label).tag(option.rawValue)
                }
            }
            Text("The shortcut opens the search panel from any application.")
                .font(.caption)
                .foregroundStyle(.secondary)
            if hotKeys.registrationFailed {
                Text("macOS is already using this shortcut. Choose another one.")
                    .font(.caption)
                    .foregroundStyle(.orange)
            }
        }
        .formStyle(.grouped)
        .frame(width: 440, height: 220)
        .onChange(of: shortcut) { _, newValue in
            guard let value = SearchShortcut(rawValue: newValue) else { return }
            if !HotKeyManager.shared.register(value), let active = HotKeyManager.shared.activeShortcut {
                shortcut = active.rawValue
            }
        }
    }
}
