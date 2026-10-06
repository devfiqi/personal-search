import SwiftUI

@main
struct PersonalSearchApp: App {
    var body: some Scene {
        WindowGroup {
            SearchView()
        }
        .defaultSize(width: 680, height: 104)
        .windowStyle(.hiddenTitleBar)
    }
}

