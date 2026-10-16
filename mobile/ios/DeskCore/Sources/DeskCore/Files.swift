// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

// The files channel (docs/protocol.md, "Files"): browse the computer's home
// folder, download a file, send one. A port of the Mac app's client
// (backend-device/internal/viewer/files.go) and the website's (files.ts),
// on the main actor with callbacks. One upload and one download at a time.

/** The "files" data channel, as the client needs it (called on the main actor). */
public protocol FilesChannel: AnyObject {
    @discardableResult func sendText(_ text: String) -> Bool
    @discardableResult func sendData(_ bytes: [UInt8]) -> Bool
    /** Bytes queued in the channel and not sent yet. */
    var bufferedAmount: UInt64 { get }
    var isOpen: Bool { get }
}

/** One entry of a folder: folders come first. */
public struct FileEntry: Equatable, Sendable, Identifiable {
    public let name: String
    public let dir: Bool
    public let size: Int64
    /** Seconds since 1970. */
    public let mtime: Int64

    public var id: String { name }

    public init(name: String, dir: Bool, size: Int64, mtime: Int64) {
        self.name = name
        self.dir = dir
        self.size = size
        self.mtime = mtime
    }
}

public struct FilesError: Error, Equatable, Sendable {
    public let message: String
    public init(_ message: String) { self.message = message }

    public static let closed = FilesError("the connection closed")
    public static let noAnswer = FilesError("the computer did not answer")
    public static let stopped = FilesError("stopped")
    public static let busy = FilesError("another transfer is running")
}

/** Where a download's pieces go (a file). */
public protocol DownloadSink: AnyObject {
    func write(_ data: Data, at offset: Int64) throws
}

/** Where an upload's bytes come from (a file). */
public protocol UploadSource: AnyObject {
    /** Up to max bytes, or an empty array at the end. */
    func read(max: Int) throws -> [UInt8]
}

/** Something running that can be stopped. */
public final class Transfer {
    fileprivate var stop: (() -> Void)?
    public func cancel() { stop?() }
}

@MainActor
public final class FilesClient {
    static let uploadChunk = 16 << 10
    static let uploadWindow: UInt64 = 1 << 20
    static let downloadChunk: Int64 = 60 << 10
    static let downloadInFlight = 16
    static let replyWaitMs = 30_000
    static let maxEntries = 5000

    private let channel: FilesChannel
    private let scheduler: Scheduler
    private var seq = 0
    private var waiting: [String: (JSONObject) -> Void] = [:]
    private var binary: ((Data) -> Void)?
    private var drained: (() -> Void)?
    private var uploading = false
    private var downloading = false
    private var closed = false
    private var closers: [String: () -> Void] = [:]

    public init(channel: FilesChannel, scheduler: Scheduler) {
        self.channel = channel
        self.scheduler = scheduler
    }

    // MARK: - From the channel

    public func onText(_ text: String) {
        guard let m = JSONText.parseObject(text) else { return }
        let id = m["id"].str(40)
        waiting[id]?(m)
    }

    public func onBinary(_ data: Data) { binary?(data) }

    /** The channel's queue went down: an upload may go on. */
    public func onBufferedLow() {
        let d = drained
        drained = nil
        d?()
    }

    public func onClosed() {
        guard !closed else { return }
        closed = true
        for (_, close) in closers { close() }
        closers = [:]
        waiting = [:]
        binary = nil
    }

    // MARK: - Browsing

    /** The entries of a folder, relative to the computer's home folder ("" is the home folder). */
    public func list(_ path: String, done: @escaping (Result<[FileEntry], FilesError>) -> Void) {
        let id = open("l")
        var timer: CancelHandle?
        let finish: (Result<[FileEntry], FilesError>) -> Void = { [weak self] r in
            timer?.cancel()
            self?.forget(id)
            done(r)
        }
        closers[id] = { finish(.failure(.closed)) }
        waiting[id] = { m in
            switch m["type"].str(30) {
            case "listing":
                let entries = m["entries"].arr.prefix(FilesClient.maxEntries).compactMap { e -> FileEntry? in
                    let o = Optional(e)
                    let name = o?["name"].str(255) ?? ""
                    guard !name.isEmpty else { return nil }
                    return FileEntry(name: name, dir: o?["dir"].isTrue ?? false,
                                     size: max(0, o?["size"].int64 ?? 0), mtime: o?["mtime"].int64 ?? 0)
                }
                finish(.success(entries))
            case "files_error":
                finish(.failure(FilesError(m["error"].str(200))))
            default:
                break
            }
        }
        timer = scheduler.after(ms: FilesClient.replyWaitMs) { finish(.failure(.noAnswer)) }
        if !send(.obj(("type", "list"), ("id", .string(id)), ("path", .string(path)))) { finish(.failure(.closed)) }
    }

    // MARK: - Downloads

    /**
     * Downloads path into sink: ready gets the file's name and size first
     * (to make the file), then progress, then done with the name.
     */
    @discardableResult
    public func download(_ path: String, ready: @escaping (_ name: String, _ size: Int64) throws -> DownloadSink,
                         progress: @escaping (_ got: Int64, _ size: Int64) -> Void,
                         done: @escaping (Result<String, FilesError>) -> Void) -> Transfer {
        let t = Transfer()
        guard !downloading else {
            done(.failure(.busy))
            return t
        }
        downloading = true
        let id = open("d")
        var timer: CancelHandle?
        var finished = false
        var sink: DownloadSink?
        var name = ""
        var size: Int64 = 0
        var next: Int64 = 0
        var got: Int64 = 0
        var inFlight = 0
        let finish: (Result<String, FilesError>) -> Void = { [weak self] r in
            guard !finished else { return }
            finished = true
            timer?.cancel()
            self?.binary = nil
            self?.downloading = false
            self?.forget(id)
            done(r)
        }
        let rearm = { [weak self] in
            timer?.cancel()
            timer = self?.scheduler.after(ms: FilesClient.replyWaitMs) { finish(.failure(.noAnswer)) }
        }
        let ask = { [weak self] in
            guard let self else { return }
            while inFlight < FilesClient.downloadInFlight && next < size {
                let n = min(FilesClient.downloadChunk, size - next)
                self.send(.obj(("type", "read"), ("id", .string(id)), ("offset", .number(Double(next))), ("length", .number(Double(n)))))
                next += n
                inFlight += 1
            }
        }
        t.stop = { [weak self] in
            self?.send(.obj(("type", "cancel"), ("id", .string(id))))
            finish(.failure(.stopped))
        }
        closers[id] = { finish(.failure(.closed)) }
        waiting[id] = { [weak self] m in
            switch m["type"].str(30) {
            case "download_ready":
                name = FilesClient.safeName(m["name"].str(255))
                if name.isEmpty { name = FilesClient.safeName((path as NSString).lastPathComponent) }
                size = max(0, JSON.object(m)["size"].int64)
                do {
                    sink = try ready(name, size)
                } catch {
                    self?.send(.obj(("type", "cancel"), ("id", .string(id))))
                    finish(.failure(FilesError(error.localizedDescription)))
                    return
                }
                if size == 0 {
                    self?.send(.obj(("type", "done"), ("id", .string(id))))
                    finish(.success(name))
                    return
                }
                rearm()
                ask()
            case "download_error", "files_error":
                finish(.failure(FilesError(m["error"].str(200))))
            default:
                break
            }
        }
        binary = { [weak self] data in
            guard data.count >= 8, let sink else { return }
            let off = data.prefix(8).reduce(Int64(0)) { $0 << 8 | Int64($1) }
            let piece = data.dropFirst(8)
            // Non-trapping bounds: off can be any Int64 (even Int64.max), so
            // off + piece.count would overflow and abort the app. size >= 0
            // and 0 <= off <= size make size - off safe.
            guard off >= 0, off <= size, size - off >= Int64(piece.count) else {
                self?.send(.obj(("type", "cancel"), ("id", .string(id))))
                finish(.failure(FilesError("the computer sent a bad piece")))
                return
            }
            do {
                try sink.write(Data(piece), at: off)
            } catch {
                self?.send(.obj(("type", "cancel"), ("id", .string(id))))
                finish(.failure(FilesError(error.localizedDescription)))
                return
            }
            got += Int64(piece.count)
            inFlight -= 1
            progress(got, size)
            if got >= size {
                self?.send(.obj(("type", "done"), ("id", .string(id))))
                finish(.success(name))
                return
            }
            rearm()
            ask()
        }
        timer = scheduler.after(ms: FilesClient.replyWaitMs) { finish(.failure(.noAnswer)) }
        if !send(.obj(("type", "download"), ("id", .string(id)), ("path", .string(path)))) { finish(.failure(.closed)) }
        return t
    }

    // MARK: - Uploads

    /** Sends a file: done gets the name the computer saved it as (in Downloads). */
    @discardableResult
    public func upload(name: String, size: Int64, from source: UploadSource,
                       progress: @escaping (_ sent: Int64, _ size: Int64) -> Void,
                       done: @escaping (Result<String, FilesError>) -> Void) -> Transfer {
        let t = Transfer()
        guard !uploading else {
            done(.failure(.busy))
            return t
        }
        uploading = true
        let id = open("u")
        var timer: CancelHandle?
        var finished = false
        var sent: Int64 = 0
        var ended = false
        let finish: (Result<String, FilesError>) -> Void = { [weak self] r in
            guard !finished else { return }
            finished = true
            timer?.cancel()
            self?.drained = nil
            self?.uploading = false
            self?.forget(id)
            done(r)
        }
        t.stop = { [weak self] in
            self?.send(.obj(("type", "abort"), ("id", .string(id))))
            finish(.failure(.stopped))
        }
        closers[id] = { finish(.failure(.closed)) }
        waiting[id] = { m in
            guard m["type"].str(30) == "upload_result" || m["type"].str(30) == "files_error" else { return }
            if m["ok"].isTrue {
                let saved = m["name"].str(255)
                finish(.success(saved.isEmpty ? name : saved))
            } else {
                finish(.failure(FilesError(m["error"].str(200))))
            }
        }
        // Sends while the channel's queue is under the window, then waits for it to drain.
        var pump: (() -> Void)!
        pump = { [weak self] in
            guard let self, !finished else { return }
            while !ended {
                guard self.channel.isOpen else {
                    finish(.failure(.closed))
                    return
                }
                if self.channel.bufferedAmount > FilesClient.uploadWindow {
                    self.drained = pump
                    // In case the low mark is never reported.
                    self.scheduler.after(ms: 1000) { [weak self] in
                        if let d = self?.drained, !finished { self?.drained = nil; d() }
                    }
                    return
                }
                if sent >= size {
                    ended = true
                    break
                }
                let chunk: [UInt8]
                do {
                    chunk = try source.read(max: FilesClient.uploadChunk)
                } catch {
                    self.send(.obj(("type", "abort"), ("id", .string(id))))
                    finish(.failure(FilesError(error.localizedDescription)))
                    return
                }
                if chunk.isEmpty {
                    ended = true
                    break
                }
                guard self.channel.sendData(chunk) else {
                    finish(.failure(.closed))
                    return
                }
                sent += Int64(chunk.count)
                progress(sent, size)
            }
            self.send(.obj(("type", "end"), ("id", .string(id))))
            timer = self.scheduler.after(ms: FilesClient.replyWaitMs) { finish(.failure(.noAnswer)) }
        }
        let safe = FilesClient.safeName(name)
        guard send(.obj(("type", "begin"), ("id", .string(id)), ("name", .string(safe.isEmpty ? "file" : safe)), ("size", .number(Double(size))))) else {
            finish(.failure(.closed))
            return t
        }
        pump()
        return t
    }

    // MARK: - Helpers

    private func open(_ prefix: String) -> String {
        seq += 1
        return "\(prefix)\(seq)"
    }

    private func forget(_ id: String) {
        waiting[id] = nil
        closers[id] = nil
    }

    @discardableResult
    private func send(_ m: JSON) -> Bool {
        guard !closed, channel.isOpen else { return false }
        return channel.sendText(m.text)
    }

    /** A name without any folder, control characters or leading dots ("" when nothing is left). */
    public static func safeName(_ name: String) -> String {
        let last = name.replacingOccurrences(of: "\\", with: "/").split(separator: "/").last.map(String.init) ?? ""
        let clean = String(last.unicodeScalars.filter { $0.value >= 0x20 && $0.value != 0x7F && $0 != ":" })
        return String(clean.drop { $0 == "." })
    }
}
