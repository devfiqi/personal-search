import AppKit
import SwiftUI

@MainActor
enum SearchWindow {
    static let identifier = NSUserInterfaceItemIdentifier("personal-search")

    static func configure(_ window: NSWindow) {
        window.identifier = identifier
        window.level = .floating
        window.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        window.isReleasedWhenClosed = false
        window.titlebarAppearsTransparent = true
        window.title = "Personal Search"
        window.delegate = SearchWindowDelegate.shared
    }

    static func show() {
        NSApp.activate()
        let window = NSApp.windows.first { $0.identifier == identifier }
            ?? NSApp.windows.first { !($0 is NSPanel) && $0.identifier != NSUserInterfaceItemIdentifier("settings") }
        guard let window else { return }
        configure(window)
        window.makeKeyAndOrderFront(nil)
    }
}

@MainActor
final class SearchWindowDelegate: NSObject, NSWindowDelegate {
    static let shared = SearchWindowDelegate()

    func windowShouldClose(_ sender: NSWindow) -> Bool {
        sender.orderOut(nil)
        return false
    }
}

struct SearchWindowAnchor: NSViewRepresentable {
    var onReturn: () -> Void

    func makeNSView(context: Context) -> SearchWindowView {
        let view = SearchWindowView()
        view.onReturn = onReturn
        return view
    }

    func updateNSView(_ nsView: SearchWindowView, context: Context) {
        nsView.onReturn = onReturn
        nsView.configureIfNeeded()
    }
}

final class SearchWindowView: NSView {
    var onReturn: () -> Void = {}
    private var monitor: Any?

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        guard window != nil else {
            removeMonitor()
            return
        }
        configureIfNeeded()
        installMonitorIfNeeded()
    }

    func configureIfNeeded() {
        guard let window else { return }
        SearchWindow.configure(window)
    }

    private func installMonitorIfNeeded() {
        guard monitor == nil else { return }
        monitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] event in
            self?.handle(event) ?? event
        }
    }

    private func handle(_ event: NSEvent) -> NSEvent? {
        guard let window, event.window === window, window.attachedSheet == nil else { return event }
        if event.keyCode == 53 {
            window.orderOut(nil)
            return nil
        }
        let modifiers = event.modifierFlags.intersection([.command, .option, .control])
        if (event.keyCode == 36 || event.keyCode == 76), modifiers.isEmpty {
            onReturn()
            return nil
        }
        return event
    }

    private func removeMonitor() {
        if let monitor {
            NSEvent.removeMonitor(monitor)
            self.monitor = nil
        }
    }
}
