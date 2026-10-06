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
final class HotKeyManager {
    static let shared = HotKeyManager()
    private var hotKey: EventHotKeyRef?
    private var handler: EventHandlerRef?

    private init() {
        var eventType = EventTypeSpec(eventClass: OSType(kEventClassKeyboard), eventKind: UInt32(kEventHotKeyPressed))
        InstallEventHandler(
            GetApplicationEventTarget(),
            { _, _, _ in
                Task { @MainActor in
                    NSApp.activate(ignoringOtherApps: true)
                    NSApp.windows.first(where: { !($0 is NSPanel) })?.makeKeyAndOrderFront(nil)
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
        let raw = UserDefaults.standard.string(forKey: "searchShortcut") ?? SearchShortcut.commandShiftSpace.rawValue
        register(SearchShortcut(rawValue: raw) ?? .commandShiftSpace)
    }

    func register(_ shortcut: SearchShortcut) {
        if let hotKey {
            UnregisterEventHotKey(hotKey)
            self.hotKey = nil
        }
        let identifier = EventHotKeyID(signature: OSType(0x50535243), id: 1)
        RegisterEventHotKey(
            UInt32(kVK_Space),
            shortcut.modifiers,
            identifier,
            GetApplicationEventTarget(),
            0,
            &hotKey
        )
    }
}

struct ShortcutSettingsView: View {
    @AppStorage("searchShortcut") private var shortcut = SearchShortcut.commandShiftSpace.rawValue

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
        }
        .formStyle(.grouped)
        .frame(width: 440, height: 180)
        .onChange(of: shortcut) { _, newValue in
            if let value = SearchShortcut(rawValue: newValue) { HotKeyManager.shared.register(value) }
        }
    }
}
