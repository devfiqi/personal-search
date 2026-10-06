import SwiftUI

struct FolderManagementView: View {
    @EnvironmentObject private var model: SearchViewModel
    @Environment(\.dismiss) private var dismiss
    @State private var folderToRemove: IndexedFolder?

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("Indexed Folders").font(.title2.bold())
                Spacer()
                Button("Done") { dismiss() }.keyboardShortcut(.defaultAction)
            }
            .padding(20)
            Divider()

            if model.folders.isEmpty {
                ContentUnavailableView(
                    "No Indexed Folders",
                    systemImage: "folder",
                    description: Text("Add a folder to make its supported documents searchable.")
                )
            } else {
                List(model.folders) { folder in
                    HStack(spacing: 12) {
                        Image(systemName: "folder.fill").foregroundStyle(.tint)
                        VStack(alignment: .leading, spacing: 3) {
                            Text(folder.displayName).font(.headline)
                            Text(folder.path)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                                .lineLimit(1)
                                .truncationMode(.middle)
                        }
                        Spacer()
                        Button(role: .destructive) { folderToRemove = folder } label: {
                            Image(systemName: "minus.circle")
                        }
                        .buttonStyle(.plain)
                        .help("Stop indexing this folder")
                        .accessibilityLabel("Remove \(folder.displayName)")
                    }
                    .padding(.vertical, 5)
                }
            }

            Divider()
            HStack {
                Button("Add Folder…") { Task { await model.chooseFolder() } }
                Spacer()
                Text("Removing a folder deletes only its derived index data.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            .padding(20)
        }
        .frame(width: 580, height: 400)
        .confirmationDialog(
            "Stop indexing \(folderToRemove?.displayName ?? "this folder")?",
            isPresented: Binding(
                get: { folderToRemove != nil },
                set: { if !$0 { folderToRemove = nil } }
            )
        ) {
            Button("Remove From Index", role: .destructive) {
                if let folderToRemove { Task { await model.removeFolder(folderToRemove) } }
                folderToRemove = nil
            }
            Button("Cancel", role: .cancel) { folderToRemove = nil }
        } message: {
            Text("Original files will not be deleted or changed.")
        }
    }
}

struct FailureListView: View {
    @EnvironmentObject private var model: SearchViewModel
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("Extraction Issues").font(.title2.bold())
                Spacer()
                Button("Done") { dismiss() }.keyboardShortcut(.defaultAction)
            }
            .padding(20)
            Divider()
            List(model.failures) { failure in
                HStack(alignment: .top, spacing: 12) {
                    Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.orange)
                    VStack(alignment: .leading, spacing: 4) {
                        Text(failure.name).font(.headline)
                        Text(failure.message).font(.callout).foregroundStyle(.secondary)
                        Text(failure.path)
                            .font(.caption)
                            .foregroundStyle(.tertiary)
                            .lineLimit(1)
                            .truncationMode(.middle)
                    }
                }
                .padding(.vertical, 5)
            }
        }
        .frame(width: 600, height: 420)
    }
}

struct GmailManagementView: View {
    @EnvironmentObject private var model: SearchViewModel
    @Environment(\.dismiss) private var dismiss
    @State private var clientID = ""
    @State private var accountToRemove: GmailAccount?

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("Gmail").font(.title2.bold())
                Spacer()
                Button("Done") { dismiss() }.keyboardShortcut(.defaultAction)
            }
            .padding(20)
            Divider()
            List {
                if model.gmailAccounts.isEmpty {
                    ContentUnavailableView(
                        "No Gmail Account Connected",
                        systemImage: "envelope",
                        description: Text("Connect a Gmail account to make its mail searchable on this Mac.")
                    )
                    .listRowSeparator(.hidden)
                } else {
                    Section("Connected accounts") {
                        ForEach(model.gmailAccounts) { account in
                            HStack {
                                Image(systemName: "checkmark.circle.fill").foregroundStyle(.green)
                                Text(account.email).textSelection(.enabled)
                                Spacer()
                                Button(role: .destructive) { accountToRemove = account } label: {
                                    Image(systemName: "minus.circle")
                                }
                                .buttonStyle(.plain)
                                .accessibilityLabel("Disconnect \(account.email)")
                            }
                        }
                    }
                }
                Section("Connect Gmail") {
                    TextField("Google Desktop OAuth Client ID", text: $clientID)
                        .textFieldStyle(.roundedBorder)
                        .accessibilityLabel("Google Desktop OAuth Client ID")
                    Text("Use a Desktop OAuth client ID from your Google Cloud project. Personal Search requests read-only Gmail access and stores its refresh token only in your macOS Keychain.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    if let notice = model.notice {
                        Text(notice).font(.caption).foregroundStyle(.red)
                    }
                    Button {
                        Task { await model.connectGmail(clientID: clientID.trimmingCharacters(in: .whitespacesAndNewlines)) }
                    } label: {
                        if model.connectingGmail { ProgressView().controlSize(.small) }
                        Text(model.connectingGmail ? "Connecting…" : "Connect Gmail")
                    }
                    .disabled(model.connectingGmail || clientID.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
            }
        }
        .frame(width: 600, height: 440)
        .confirmationDialog(
            "Disconnect \(accountToRemove?.email ?? "this Gmail account")?",
            isPresented: Binding(get: { accountToRemove != nil }, set: { if !$0 { accountToRemove = nil } })
        ) {
            Button("Disconnect", role: .destructive) {
                if let accountToRemove { Task { await model.removeGmail(accountToRemove) } }
                accountToRemove = nil
            }
            Button("Cancel", role: .cancel) { accountToRemove = nil }
        } message: {
            Text("The local Gmail index for this account will be removed as email syncing is added. Gmail itself will not be changed.")
        }
    }
}
