import AppKit
import Darwin
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
    private var showSignal: DispatchSourceSignal?

    func applicationWillFinishLaunching(_ notification: Notification) {
        guard AppInstance.claim() else {
            DispatchQueue.main.async { NSApp.terminate(nil) }
            return
        }
        showSignal = AppInstance.showSignal { SearchWindow.show() }
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        hotKeyManager.registerSavedShortcut()
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        SearchWindow.show()
        return false
    }

    func applicationWillTerminate(_ notification: Notification) {
        CoreProcessController.shared.stop()
    }
}

@MainActor
enum AppInstance {
    private static var lockFD: Int32 = -1
    private(set) static var isPrimary = true

    @discardableResult
    static func claim() -> Bool {
        if lockFD >= 0 { return isPrimary }
        do {
            let url = try CoreProcessController.supportDirectory().appendingPathComponent("instance.lock")
            let fd = open(url.path, O_CREAT | O_RDWR, 0o600)
            guard fd >= 0 else { return true }
            if flock(fd, LOCK_EX | LOCK_NB) != 0 {
                close(fd)
                signalPrimary(at: url)
                isPrimary = false
                return false
            }
            _ = ftruncate(fd, 0)
            _ = lseek(fd, 0, SEEK_SET)
            _ = fcntl(fd, F_SETFD, FD_CLOEXEC)
            let pid = Data("\(getpid())\n".utf8)
            _ = pid.withUnsafeBytes { bytes in
                write(fd, bytes.baseAddress, bytes.count)
            }
            lockFD = fd
            isPrimary = true
            return true
        } catch {
            return true
        }
    }

    static func showSignal(show: @escaping () -> Void) -> DispatchSourceSignal {
        signal(SIGUSR1, SIG_IGN)
        let source = DispatchSource.makeSignalSource(signal: SIGUSR1, queue: .main)
        source.setEventHandler(handler: show)
        source.resume()
        return source
    }

    private static func signalPrimary(at url: URL) {
        for _ in 0..<10 {
            guard let text = try? String(contentsOf: url, encoding: .utf8),
                  let pid = pid_t(text.trimmingCharacters(in: .whitespacesAndNewlines)),
                  pid > 0, pid != getpid() else {
                usleep(20_000)
                continue
            }
            kill(pid, SIGUSR1)
            NSRunningApplication(processIdentifier: pid)?.activate(options: [.activateAllWindows])
            return
        }
    }
}
