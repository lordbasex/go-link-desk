// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import DeskCore

/** The files channel's client against a fake channel that answers as the computer does. */
@MainActor
final class FilesTests: XCTestCase {
    final class Channel: FilesChannel {
        var texts: [JSONObject] = []
        var data: [[UInt8]] = []
        var bufferedAmount: UInt64 = 0
        var isOpen = true
        func sendText(_ text: String) -> Bool { texts.append(JSONText.parseObject(text)!); return isOpen }
        func sendData(_ bytes: [UInt8]) -> Bool { data.append(bytes); return isOpen }
    }

    final class Sink: DownloadSink {
        var bytes = [UInt8](repeating: 0, count: 0)
        func write(_ data: Data, at offset: Int64) throws {
            let end = Int(offset) + data.count
            if bytes.count < end { bytes += [UInt8](repeating: 0, count: end - bytes.count) }
            bytes.replaceSubrange(Int(offset)..<end, with: data)
        }
    }

    final class Source: UploadSource {
        var rest: [UInt8]
        init(_ b: [UInt8]) { rest = b }
        func read(max: Int) throws -> [UInt8] {
            let n = min(max, rest.count)
            defer { rest.removeFirst(n) }
            return Array(rest.prefix(n))
        }
    }

    func testList() {
        let ch = Channel(), clock = ManualScheduler()
        let f = FilesClient(channel: ch, scheduler: clock)
        var got: [FileEntry] = []
        f.list("Documents") { if case let .success(e) = $0 { got = e } }
        XCTAssertEqual(ch.texts.last, obj(#"{"type":"list","id":"l1","path":"Documents"}"#))
        f.onText(#"{"type":"listing","id":"l1","path":"Documents","entries":[{"name":"a","dir":true,"size":0,"mtime":1},{"name":"b.txt","dir":false,"size":5,"mtime":2}]}"#)
        XCTAssertEqual(got.map(\.name), ["a", "b.txt"])
        XCTAssertTrue(got[0].dir)

        var err: FilesError?
        f.list("..") { if case let .failure(e) = $0 { err = e } }
        f.onText(#"{"type":"files_error","id":"l2","error":"not allowed"}"#)
        XCTAssertEqual(err, FilesError("not allowed"))

        var late: FilesError?
        f.list("x") { if case let .failure(e) = $0 { late = e } }
        clock.advance(by: FilesClient.replyWaitMs)
        XCTAssertEqual(late, .noAnswer)
    }

    func testDownloadInPiecesOutOfOrder() {
        let ch = Channel(), clock = ManualScheduler()
        let f = FilesClient(channel: ch, scheduler: clock)
        let sink = Sink()
        var result: Result<String, FilesError>?
        var progress: [Int64] = []
        f.download("Documents/x.bin", ready: { name, size in
            XCTAssertEqual(name, "x.bin")
            XCTAssertEqual(size, 70_000)
            return sink
        }, progress: { got, _ in progress.append(got) }, done: { result = $0 })
        XCTAssertEqual(ch.texts.last, obj(#"{"type":"download","id":"d1","path":"Documents/x.bin"}"#))
        f.onText(#"{"type":"download_ready","id":"d1","name":"../x.bin","size":70000}"#)
        let reads = ch.texts.filter { $0["type"] == "read" }
        XCTAssertEqual(reads.count, 2) // 60 KiB and the rest
        func piece(_ off: Int, _ n: Int) -> Data {
            var d = Data((0..<8).map { UInt8((off >> (8 * (7 - $0))) & 0xFF) })
            d.append(contentsOf: (0..<n).map { UInt8((off + $0) % 251) })
            return d
        }
        f.onBinary(piece(61_440, 70_000 - 61_440))
        f.onBinary(piece(0, 61_440))
        XCTAssertEqual(result, .success("x.bin"))
        XCTAssertEqual(ch.texts.last?["type"], "done")
        XCTAssertEqual(sink.bytes.count, 70_000)
        XCTAssertEqual(sink.bytes[61_440], UInt8(61_440 % 251))
        XCTAssertEqual(progress.last, 70_000)

        // A piece outside the file stops it.
        var bad: Result<String, FilesError>?
        f.download("y", ready: { _, _ in Sink() }, progress: { _, _ in }, done: { bad = $0 })
        f.onText(#"{"type":"download_ready","id":"d2","name":"y","size":10}"#)
        f.onBinary(piece(5, 10))
        XCTAssertEqual(bad, .failure(FilesError("the computer sent a bad piece")))
        XCTAssertEqual(ch.texts.last?["type"], "cancel")
    }

    // A piece whose 8-byte offset is Int64.max no longer aborts the app:
    // the old bounds check added the payload length and trapped on signed
    // overflow; now it is refused as a bad piece.
    func testADownloadRejectsAnOverflowingOffset() {
        let ch = Channel(), clock = ManualScheduler()
        let f = FilesClient(channel: ch, scheduler: clock)
        var bad: Result<String, FilesError>?
        f.download("z", ready: { _, _ in Sink() }, progress: { _, _ in }, done: { bad = $0 })
        f.onText(#"{"type":"download_ready","id":"d1","name":"z","size":10}"#)
        var d = Data([0x7F, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF]) // offset Int64.max
        d.append(0)
        f.onBinary(d)
        XCTAssertEqual(bad, .failure(FilesError("the computer sent a bad piece")))
        XCTAssertEqual(ch.texts.last?["type"], "cancel")
    }

    func testUploadWaitsForTheChannelToDrain() {
        let ch = Channel(), clock = ManualScheduler()
        let f = FilesClient(channel: ch, scheduler: clock)
        let bytes = [UInt8](repeating: 7, count: 40_000)
        var result: Result<String, FilesError>?
        ch.bufferedAmount = FilesClient.uploadWindow + 1
        f.upload(name: "dir/.photo.jpg", size: Int64(bytes.count), from: Source(bytes), progress: { _, _ in }, done: { result = $0 })
        XCTAssertEqual(ch.texts.first, obj(#"{"type":"begin","id":"u1","name":"photo.jpg","size":40000}"#))
        XCTAssertTrue(ch.data.isEmpty) // the queue is full
        ch.bufferedAmount = 0
        f.onBufferedLow()
        XCTAssertEqual(ch.data.map(\.count), [16_384, 16_384, 7232])
        XCTAssertEqual(ch.texts.last, obj(#"{"type":"end","id":"u1"}"#))
        f.onText(#"{"type":"upload_result","id":"u1","ok":true,"name":"photo (2).jpg"}"#)
        XCTAssertEqual(result, .success("photo (2).jpg"))

        // One upload at a time; a closed channel fails the running one.
        var a: Result<String, FilesError>?, b: Result<String, FilesError>?
        ch.bufferedAmount = FilesClient.uploadWindow + 1
        f.upload(name: "a", size: 3, from: Source([1, 2, 3]), progress: { _, _ in }, done: { a = $0 })
        f.upload(name: "b", size: 3, from: Source([1, 2, 3]), progress: { _, _ in }, done: { b = $0 })
        XCTAssertEqual(b, .failure(.busy))
        f.onClosed()
        XCTAssertEqual(a, .failure(.closed))
    }

    func testSafeNames() {
        XCTAssertEqual(FilesClient.safeName("../../etc/passwd"), "passwd")
        XCTAssertEqual(FilesClient.safeName("C:\\Users\\x\\a.txt"), "a.txt")
        XCTAssertEqual(FilesClient.safeName("...hidden"), "hidden")
        XCTAssertEqual(FilesClient.safeName("a\u{0}b:c"), "abc")
        XCTAssertEqual(FilesClient.safeName(".."), "")
    }
}
