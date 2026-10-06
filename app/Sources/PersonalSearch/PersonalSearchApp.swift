import AppKit
import SwiftUI

@main
struct PersonalSearchApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var model = SearchViewModel()

    var body: some Scene {
        WindowGroup {
            SearchView()
                .environmentObject(model)
                .task { await model.start() }
        }
        .defaultSize(width: 720, height: 540)
        .windowResizability(.contentSize)
        .windowStyle(.hiddenTitleBar)

        Settings {
            ShortcutSettingsView()
        }
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    private let hotKeyManager = HotKeyManager.shared
    private var windowObserver: NSObjectProtocol?

    func applicationDidFinishLaunching(_ notification: Notification) {
        hotKeyManager.registerSavedShortcut()
        windowObserver = NotificationCenter.default.addObserver(
            forName: NSWindow.didBecomeKeyNotification,
            object: nil,
            queue: .main
        ) { notification in
            guard let window = notification.object as? NSWindow else { return }
            Task { @MainActor in
                window.level = .floating
                window.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
                window.isReleasedWhenClosed = false
                window.titlebarAppearsTransparent = true
            }
        }
    }

    func applicationWillTerminate(_ notification: Notification) {
        CoreProcessController.shared.stop()
    }
}
