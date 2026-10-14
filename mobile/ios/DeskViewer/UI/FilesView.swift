// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import DeskCore
import PhotosUI
import SwiftUI

/**
 * The computer's files, as on the Mac app: its home folder to browse, a
 * tap on a file downloads it (into the Files app, On My iPhone or iPad ›
 * go-link Desk), and Send takes a file from the Files app or a photo to
 * the computer's Downloads.
 */
struct FilesView: View {
    @ObservedObject var model: FilesModel
    @Environment(\.dismiss) private var dismiss
    @State private var path: [String] = []
    @State private var importing = false
    @State private var photo: PhotosPickerItem?

    var body: some View {
        NavigationStack(path: $path) {
            FolderView(model: model, folder: "")
                .navigationDestination(for: String.self) { FolderView(model: model, folder: $0) }
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) { Button("Done") { dismiss() } }
                    ToolbarItem(placement: .primaryAction) {
                        Menu {
                            Button { importing = true } label: { Label("From Files", systemImage: "folder") }
                            PhotosPicker(selection: $photo, matching: .any(of: [.images, .videos])) {
                                Label("From Photos", systemImage: "photo")
                            }
                        } label: {
                            Label("Send", systemImage: "square.and.arrow.up")
                        }
                        .disabled(model.progress != nil)
                    }
                }
        }
        .safeAreaInset(edge: .bottom) { TransferBar(model: model) }
        .fileImporter(isPresented: $importing, allowedContentTypes: [.item]) { result in
            if case let .success(url) = result { model.upload(url, scoped: true) }
        }
        .onChange(of: photo) { _, item in
            guard let item else { return }
            photo = nil
            Task { await sendPhoto(item) }
        }
    }

    /** A photo or video goes as a copy, named by its date and kind. */
    private func sendPhoto(_ item: PhotosPickerItem) async {
        guard let data = try? await item.loadTransferable(type: Data.self) else {
            model.outcome = .failed(String(localized: "That photo cannot be read."))
            return
        }
        let ext = item.supportedContentTypes.first?.preferredFilenameExtension ?? "jpg"
        let stamp = Date().formatted(.iso8601.year().month().day().time(includingFractionalSeconds: false)).replacingOccurrences(of: ":", with: "-")
        let name = "Photo \(stamp).\(ext)"
        let tmp = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        do {
            try data.write(to: tmp)
        } catch {
            model.outcome = .failed(error.localizedDescription)
            return
        }
        model.upload(tmp, name: name)
    }
}

/** One folder of the computer. */
private struct FolderView: View {
    @ObservedObject var model: FilesModel
    let folder: String
    @State private var entries: [FileEntry]?
    @State private var error: String?

    var body: some View {
        Group {
            if let entries {
                if entries.isEmpty {
                    ContentUnavailableView("This folder is empty", systemImage: "folder")
                } else {
                    List(entries) { e in
                        if e.dir {
                            NavigationLink(value: join(e.name)) { row(e) }
                        } else {
                            Button { model.download(join(e.name)) } label: { row(e) }
                                .disabled(model.progress != nil)
                        }
                    }
                    .listStyle(.plain)
                }
            } else if let error {
                ContentUnavailableView {
                    Label("Cannot open this folder", systemImage: "exclamationmark.triangle")
                } description: {
                    Text(error)
                } actions: {
                    Button("Try again") { load() }
                }
            } else {
                ProgressView()
            }
        }
        .navigationTitle(folder.isEmpty ? String(localized: "The computer's files") : (folder as NSString).lastPathComponent)
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { load() }
        .onAppear { if entries == nil { load() } }
    }

    private func join(_ name: String) -> String { folder.isEmpty ? name : folder + "/" + name }

    private func load() {
        error = nil
        model.list(folder) { r in
            switch r {
            case let .success(e): entries = e
            case let .failure(e): error = e.message
            }
        }
    }

    private func row(_ e: FileEntry) -> some View {
        HStack(spacing: 12) {
            Image(systemName: e.dir ? "folder.fill" : "doc")
                .foregroundStyle(e.dir ? Theme.accent : .secondary)
                .frame(width: 26)
            VStack(alignment: .leading, spacing: 2) {
                Text(e.name).lineLimit(1)
                Text(detail(e)).font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            if !e.dir { Image(systemName: "arrow.down.circle").foregroundStyle(Theme.accent) }
        }
        .contentShape(Rectangle())
    }

    private func detail(_ e: FileEntry) -> String {
        let date = Date(timeIntervalSince1970: TimeInterval(e.mtime)).formatted(date: .abbreviated, time: .shortened)
        return e.dir ? date : ByteCountFormatter.string(fromByteCount: e.size, countStyle: .file) + " · " + date
    }
}

/** The transfer running, or what came of the last one. */
private struct TransferBar: View {
    @ObservedObject var model: FilesModel

    var body: some View {
        if let p = model.progress {
            HStack(spacing: 12) {
                Image(systemName: p.upload ? "arrow.up.circle" : "arrow.down.circle").foregroundStyle(Theme.accent)
                VStack(alignment: .leading, spacing: 4) {
                    Text(p.name).font(.footnote).lineLimit(1)
                    ProgressView(value: p.fraction)
                    Text(verbatim: "\(ByteCountFormatter.string(fromByteCount: p.done, countStyle: .file)) / \(ByteCountFormatter.string(fromByteCount: p.size, countStyle: .file))")
                        .font(.caption2).foregroundStyle(.secondary)
                }
                Button("Cancel", role: .cancel) { model.cancel() }.font(.footnote)
            }
            .padding(12)
            .background(.bar)
        } else if let o = model.outcome {
            HStack(spacing: 12) {
                switch o {
                case let .downloaded(url):
                    Image(systemName: "checkmark.circle.fill").foregroundStyle(.green)
                    Text(verbatim: String(localized: "Saved in Files › go-link Desk:") + " " + url.lastPathComponent).font(.footnote).lineLimit(2)
                    Spacer()
                    ShareLink(item: url) { Image(systemName: "square.and.arrow.up") }
                case let .uploaded(name):
                    Image(systemName: "checkmark.circle.fill").foregroundStyle(.green)
                    Text(verbatim: String(localized: "Sent to the computer's Downloads:") + " " + name).font(.footnote).lineLimit(2)
                    Spacer()
                case let .failed(why):
                    Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.orange)
                    Text(why).font(.footnote).lineLimit(2)
                    Spacer()
                }
                Button { model.outcome = nil } label: { Image(systemName: "xmark") }.foregroundStyle(.secondary)
            }
            .padding(12)
            .background(.bar)
        }
    }
}
