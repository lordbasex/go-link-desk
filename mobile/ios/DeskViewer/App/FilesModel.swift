// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import DeskCore
import Foundation

/**
 * The computer's files as the Files sheet shows them: a folder's
 * entries, the transfer running (at most one download and one upload), and
 * what came of the last one. Downloads go to this app's Documents folder,
 * which the Files app shows as On My iPhone (or iPad) › go-link Desk.
 */
@MainActor
final class FilesModel: ObservableObject {
    struct Progress: Equatable {
        let name: String
        let upload: Bool
        var done: Int64
        var size: Int64
        var fraction: Double { size > 0 ? Double(done) / Double(size) : 0 }
    }

    enum Outcome: Equatable {
        case downloaded(URL)
        case uploaded(String)
        case failed(String)
    }

    let client: FilesClient
    @Published private(set) var progress: Progress?
    @Published var outcome: Outcome?
    private var transfer: Transfer?

    init(client: FilesClient) {
        self.client = client
    }

    func list(_ path: String, done: @escaping (Result<[FileEntry], FilesError>) -> Void) {
        client.list(path, done: done)
    }

    static var downloads: URL {
        FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
    }

    func download(_ path: String) {
        guard progress == nil else { return }
        let dir = FilesModel.downloads
        var tmp: URL?
        var handle: FileSink?
        let name = (path as NSString).lastPathComponent
        progress = Progress(name: name, upload: false, done: 0, size: 0)
        transfer = client.download(path, ready: { name, size in
            let t = dir.appendingPathComponent(".\(UUID().uuidString).part")
            FileManager.default.createFile(atPath: t.path, contents: nil)
            let sink = try FileSink(url: t)
            tmp = t
            handle = sink
            return sink
        }, progress: { [weak self] got, size in
            self?.progress?.done = got
            self?.progress?.size = size
        }, done: { [weak self] result in
            guard let self else { return }
            handle?.close()
            self.progress = nil
            self.transfer = nil
            switch result {
            case let .success(saved):
                guard let tmp else { return }
                let dest = FilesModel.freeURL(dir, saved)
                do {
                    try FileManager.default.moveItem(at: tmp, to: dest)
                    self.outcome = .downloaded(dest)
                } catch {
                    try? FileManager.default.removeItem(at: tmp)
                    self.outcome = .failed(error.localizedDescription)
                }
            case let .failure(e):
                if let tmp { try? FileManager.default.removeItem(at: tmp) }
                if e != .stopped { self.outcome = .failed(e.message) }
            }
        })
    }

    /** Sends a file of this device (a URL from the Files app or a copy of a photo). */
    func upload(_ url: URL, name: String? = nil, scoped: Bool = false) {
        guard progress == nil else { return }
        let access = scoped && url.startAccessingSecurityScopedResource()
        let size = (try? FileManager.default.attributesOfItem(atPath: url.path)[.size] as? NSNumber)?.int64Value ?? 0
        guard let source = try? FileSource(url: url) else {
            if access { url.stopAccessingSecurityScopedResource() }
            outcome = .failed(String(localized: "That file cannot be read."))
            return
        }
        let shown = name ?? url.lastPathComponent
        progress = Progress(name: shown, upload: true, done: 0, size: size)
        transfer = client.upload(name: shown, size: size, from: source, progress: { [weak self] sent, size in
            self?.progress?.done = sent
            self?.progress?.size = size
        }, done: { [weak self] result in
            source.close()
            if access { url.stopAccessingSecurityScopedResource() }
            guard let self else { return }
            self.progress = nil
            self.transfer = nil
            switch result {
            case let .success(saved): self.outcome = .uploaded(saved)
            case let .failure(e): if e != .stopped { self.outcome = .failed(e.message) }
            }
        })
    }

    func cancel() {
        transfer?.cancel()
    }

    /** name, or "name (2).ext" when it is taken: never over another file. */
    static func freeURL(_ dir: URL, _ name: String) -> URL {
        var u = dir.appendingPathComponent(name)
        let ext = (name as NSString).pathExtension
        let base = (name as NSString).deletingPathExtension
        var i = 2
        while FileManager.default.fileExists(atPath: u.path) {
            u = dir.appendingPathComponent(ext.isEmpty ? "\(base) (\(i))" : "\(base) (\(i)).\(ext)")
            i += 1
        }
        return u
    }
}

/** A download's file, written at the pieces' offsets. */
final class FileSink: DownloadSink {
    private let handle: FileHandle

    init(url: URL) throws {
        handle = try FileHandle(forWritingTo: url)
    }

    func write(_ data: Data, at offset: Int64) throws {
        try handle.seek(toOffset: UInt64(offset))
        try handle.write(contentsOf: data)
    }

    func close() { try? handle.close() }
}

/** An upload's file, read in order. */
final class FileSource: UploadSource {
    private let handle: FileHandle

    init(url: URL) throws {
        handle = try FileHandle(forReadingFrom: url)
    }

    func read(max: Int) throws -> [UInt8] {
        [UInt8](try handle.read(upToCount: max) ?? Data())
    }

    func close() { try? handle.close() }
}
