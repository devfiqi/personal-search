import AppKit
import SwiftUI

struct SearchView: View {
    @EnvironmentObject private var model: SearchViewModel
    @FocusState private var searchFocused: Bool
    @State private var showingFolders = false
    @State private var showingFailures = false
    @State private var showingResetConfirmation = false

    var body: some View {
        VStack(spacing: 0) {
            searchBar
            Divider().opacity(0.55)
            content
        }
        .frame(width: 720, height: 540)
        .background(.ultraThinMaterial)
        .onAppear { searchFocused = true }
        .onKeyPress(.escape) {
            NSApp.keyWindow?.orderOut(nil)
            return .handled
        }
        .onKeyPress(.return) {
            model.openTopResult()
            return model.results.isEmpty ? .ignored : .handled
        }
        .sheet(isPresented: $showingFolders) {
            FolderManagementView().environmentObject(model)
        }
        .sheet(isPresented: $showingFailures) {
            FailureListView().environmentObject(model)
        }
        .confirmationDialog(
            "Reset the local index?",
            isPresented: $showingResetConfirmation,
            titleVisibility: .visible
        ) {
            Button("Reset Index", role: .destructive) { Task { await model.reset() } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("This removes indexed text and settings. Original documents are never changed.")
        }
    }

    private var searchBar: some View {
        HStack(spacing: 12) {
            Image(systemName: "magnifyingglass")
                .font(.system(size: 19, weight: .medium))
                .foregroundStyle(.secondary)
                .accessibilityHidden(true)
            TextField("Search your documents", text: $model.query)
                .textFieldStyle(.plain)
                .font(.system(size: 21, weight: .medium, design: .rounded))
                .focused($searchFocused)
                .accessibilityLabel("Search your documents")
                .onChange(of: model.query) { _, _ in model.scheduleSearch() }
            if !model.query.isEmpty {
                Button {
                    model.query = ""
                    model.results = []
                } label: {
                    Image(systemName: "xmark.circle.fill").foregroundStyle(.tertiary)
                }
                .buttonStyle(.plain)
                .help("Clear search")
                .accessibilityLabel("Clear search")
            }
            IndexingStatusView(state: model.indexingState, connected: model.connected)
            optionsMenu
        }
        .padding(.horizontal, 20)
        .frame(height: 76)
    }

    private var optionsMenu: some View {
        Menu {
            Button("Add Folder…", systemImage: "folder.badge.plus") { Task { await model.chooseFolder() } }
            Button("Manage Folders", systemImage: "folder") { showingFolders = true }
            Divider()
            if model.indexingState.paused {
                Button("Resume Indexing", systemImage: "play.fill") { Task { await model.setPaused(false) } }
            } else {
                Button("Pause Indexing", systemImage: "pause.fill") { Task { await model.setPaused(true) } }
            }
            if model.failureCount > 0 {
                Button("Show Extraction Issues (\(model.failureCount))", systemImage: "exclamationmark.triangle") {
                    Task {
                        await model.loadFailures()
                        showingFailures = true
                    }
                }
            }
            Divider()
            SettingsLink { Text("Keyboard Shortcut…") }
            Button("Reset Local Index…", systemImage: "arrow.counterclockwise") {
                showingResetConfirmation = true
            }
        } label: {
            Image(systemName: "ellipsis.circle")
                .font(.system(size: 18))
                .frame(width: 32, height: 32)
        }
        .menuStyle(.borderlessButton)
        .menuIndicator(.hidden)
        .fixedSize()
        .accessibilityLabel("Search options")
    }

    @ViewBuilder
    private var content: some View {
        if let message = model.errorMessage {
            MessageStateView(
                icon: "exclamationmark.triangle",
                title: "Personal Search needs attention",
                message: message,
                actionTitle: "Try Again"
            ) { Task { await model.start() } }
        } else if model.folders.isEmpty {
            MessageStateView(
                icon: "folder.badge.plus",
                title: "Choose what to search",
                message: "Select a folder. Its supported documents stay private and are indexed only on this Mac.",
                actionTitle: "Choose Folder"
            ) { Task { await model.chooseFolder() } }
        } else if model.query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            ReadyStateView(folderCount: model.folders.count, indexingState: model.indexingState) {
                showingFolders = true
            }
        } else if model.searching && model.results.isEmpty {
            VStack(spacing: 12) {
                ProgressView().controlSize(.small)
                Text("Searching on this Mac…").foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .accessibilityElement(children: .combine)
        } else if model.results.isEmpty {
            MessageStateView(
                icon: "doc.text.magnifyingglass",
                title: "No matching documents",
                message: "Try fewer words or a different phrase. Quoted text searches for an exact phrase."
            )
        } else {
            ScrollView {
                LazyVStack(spacing: 6) {
                    ForEach(Array(model.results.enumerated()), id: \.element.id) { index, result in
                        ResultRow(result: result, isTopResult: index == 0) { model.open(result) }
                    }
                }
                .padding(10)
            }
            .scrollIndicators(.automatic)
        }
    }
}

private struct IndexingStatusView: View {
    let state: IndexingState
    let connected: Bool

    var body: some View {
        HStack(spacing: 6) {
            if state.indexing {
                ProgressView().controlSize(.mini)
            } else {
                Image(systemName: icon)
                    .font(.system(size: 10, weight: .semibold))
                    .foregroundStyle(state.paused ? .orange : .green)
            }
            Text(label).font(.caption.weight(.medium)).foregroundStyle(.secondary)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .background(.quaternary.opacity(0.7), in: Capsule())
        .accessibilityLabel(label)
    }

    private var label: String {
        if !connected { return "Starting" }
        if state.paused { return "Paused" }
        if state.indexing { return "Indexing" }
        return "Ready"
    }

    private var icon: String { connected ? (state.paused ? "pause.fill" : "checkmark") : "clock" }
}

private struct ResultRow: View {
    let result: SearchResult
    let isTopResult: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            HStack(alignment: .top, spacing: 14) {
                Image(systemName: result.iconName)
                    .font(.system(size: 18, weight: .medium))
                    .foregroundStyle(.tint)
                    .frame(width: 34, height: 34)
                    .background(Color.accentColor.opacity(0.11), in: RoundedRectangle(cornerRadius: 9))
                    .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: 5) {
                    HStack(alignment: .firstTextBaseline, spacing: 8) {
                        Text(result.name).font(.system(size: 15, weight: .semibold)).lineLimit(1)
                        Text(result.typeLabel)
                            .font(.caption2.weight(.semibold))
                            .foregroundStyle(.secondary)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(.quaternary, in: Capsule())
                        Spacer(minLength: 8)
                        Text(result.modifiedLabel).font(.caption).foregroundStyle(.tertiary)
                    }
                    HighlightedSnippet(value: result.snippet)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                    Text(result.path)
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                }
                if isTopResult {
                    Text("↩").font(.caption.weight(.semibold)).foregroundStyle(.tertiary)
                        .accessibilityLabel("Press Return to open")
                }
            }
            .contentShape(Rectangle())
            .padding(12)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .buttonStyle(ResultButtonStyle())
        .accessibilityLabel("\(result.name), \(result.typeLabel), \(result.modifiedLabel)")
        .accessibilityHint("Opens the original document")
    }
}

private struct ResultButtonStyle: ButtonStyle {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .background(
                configuration.isPressed ? Color.accentColor.opacity(0.13) : Color.primary.opacity(0.045),
                in: RoundedRectangle(cornerRadius: 12)
            )
            .scaleEffect(configuration.isPressed && !reduceMotion ? 0.995 : 1)
            .animation(reduceMotion ? nil : .easeOut(duration: 0.1), value: configuration.isPressed)
    }
}

private struct HighlightedSnippet: View {
    let value: String

    var body: some View { Text(attributedValue) }

    private var attributedValue: AttributedString {
        var output = AttributedString()
        var buffer = ""
        var highlighted = false
        for character in value {
            if character == "[" || character == "]" {
                append(buffer, highlighted: highlighted, to: &output)
                buffer = ""
                highlighted = character == "["
            } else {
                buffer.append(character)
            }
        }
        append(buffer, highlighted: highlighted, to: &output)
        return output
    }

    private func append(_ string: String, highlighted: Bool, to output: inout AttributedString) {
        var segment = AttributedString(string)
        if highlighted {
            segment.font = .callout.bold()
            segment.foregroundColor = .primary
        }
        output.append(segment)
    }
}

private struct MessageStateView: View {
    let icon: String
    let title: String
    let message: String
    var actionTitle: String?
    var action: (() -> Void)?

    var body: some View {
        VStack(spacing: 12) {
            Image(systemName: icon)
                .font(.system(size: 30, weight: .light))
                .foregroundStyle(.secondary)
            Text(title).font(.headline)
            Text(message)
                .font(.callout)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 390)
            if let actionTitle, let action {
                Button(actionTitle, action: action)
                    .buttonStyle(.borderedProminent)
                    .controlSize(.large)
                    .padding(.top, 4)
            }
        }
        .padding(32)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

private struct ReadyStateView: View {
    let folderCount: Int
    let indexingState: IndexingState
    let manage: () -> Void

    var body: some View {
        VStack(spacing: 12) {
            Image(systemName: indexingState.indexing ? "arrow.triangle.2.circlepath" : "checkmark.circle")
                .font(.system(size: 30, weight: .light))
                .foregroundStyle(indexingState.indexing ? Color.accentColor : Color.secondary)
            Text(indexingState.indexing ? "Building your local index" : "Ready when you are").font(.headline)
            Text("\(folderCount) \(folderCount == 1 ? "folder" : "folders") selected · Search by keyword or exact phrase")
                .font(.callout)
                .foregroundStyle(.secondary)
            Button("Manage Folders", action: manage).buttonStyle(.link)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

struct SearchViewPreviews: PreviewProvider {
    static var previews: some View {
        SearchView().environmentObject(SearchViewModel.preview)
    }
}
