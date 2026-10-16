// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

// Defensive JSON for messages that come from the network: the computer
// and the signaling server are trusted to speak the protocol, but names
// and chat are typed by people, and a custom signaling server could send
// anything. Every reader returns a safe default instead of throwing.
//
// Objects keep their keys in insertion order, so the JSON the app sends is
// byte for byte what the website sends.

public indirect enum JSON: Equatable, Sendable {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case array([JSON])
    case object(JSONObject)

    /** A field of an object, or nil. */
    public subscript(key: String) -> JSON? {
        if case let .object(o) = self { return o[key] }
        return nil
    }
}

/** A JSON object with ordered keys. */
public struct JSONObject: Equatable, Sendable, Sequence {
    public private(set) var pairs: [(String, JSON)] = []

    public init() {}

    public init(_ pairs: [(String, JSON)]) {
        for (k, v) in pairs { self[k] = v }
    }

    public subscript(key: String) -> JSON? {
        get { pairs.first(where: { $0.0 == key })?.1 }
        set {
            if let i = pairs.firstIndex(where: { $0.0 == key }) {
                if let newValue { pairs[i].1 = newValue } else { pairs.remove(at: i) }
            } else if let newValue {
                pairs.append((key, newValue))
            }
        }
    }

    public var keys: [String] { pairs.map(\.0) }

    public func makeIterator() -> IndexingIterator<[(String, JSON)]> { pairs.makeIterator() }

    public static func == (a: JSONObject, b: JSONObject) -> Bool {
        guard a.pairs.count == b.pairs.count else { return false }
        for (k, v) in a.pairs where b[k] != v { return false }
        return true
    }

    /** The compact JSON text. */
    public var text: String { JSON.object(self).text }
}

// MARK: - Building

public extension JSON {
    /** An object from ordered pairs; nil values are left out. */
    static func obj(_ pairs: (String, JSON?)...) -> JSON {
        var o = JSONObject()
        for (k, v) in pairs { if let v { o[k] = v } }
        return .object(o)
    }

    static func int(_ v: Int) -> JSON { .number(Double(v)) }
}

extension JSON: ExpressibleByStringLiteral, ExpressibleByBooleanLiteral, ExpressibleByIntegerLiteral {
    public init(stringLiteral value: String) { self = .string(value) }
    public init(booleanLiteral value: Bool) { self = .bool(value) }
    public init(integerLiteral value: Int) { self = .number(Double(value)) }
}

// MARK: - Text

public extension JSON {
    /** Compact JSON text, like kotlinx.serialization's toString(). */
    var text: String {
        var out = ""
        write(into: &out)
        return out
    }

    private func write(into out: inout String) {
        switch self {
        case .null: out += "null"
        case let .bool(b): out += b ? "true" : "false"
        case let .number(d): out += JSON.format(d)
        case let .string(s): JSON.quote(s, into: &out)
        case let .array(a):
            out += "["
            for (i, v) in a.enumerated() {
                if i > 0 { out += "," }
                v.write(into: &out)
            }
            out += "]"
        case let .object(o):
            out += "{"
            for (i, (k, v)) in o.pairs.enumerated() {
                if i > 0 { out += "," }
                JSON.quote(k, into: &out)
                out += ":"
                v.write(into: &out)
            }
            out += "}"
        }
    }

    private static func format(_ d: Double) -> String {
        guard d.isFinite else { return "null" }
        if d == d.rounded(), abs(d) < 1e15 { return String(Int64(d)) }
        return String(d)
    }

    private static func quote(_ s: String, into out: inout String) {
        out += "\""
        for u in s.unicodeScalars {
            switch u {
            case "\"": out += "\\\""
            case "\\": out += "\\\\"
            case "\n": out += "\\n"
            case "\r": out += "\\r"
            case "\t": out += "\\t"
            case "\u{08}": out += "\\b"
            case "\u{0C}": out += "\\f"
            default:
                if u.value < 0x20 {
                    out += String(format: "\\u%04x", u.value)
                } else {
                    out.unicodeScalars.append(u)
                }
            }
        }
        out += "\""
    }
}

// MARK: - Parsing

public enum JSONText {
    /** Parses text as a JSON object, or nil for anything else. */
    public static func parseObject(_ text: String) -> JSONObject? {
        guard case let .object(o)? = parse(text) else { return nil }
        return o
    }

    /** Parses any JSON value, or nil when the text is not JSON. */
    public static func parse(_ text: String) -> JSON? {
        guard let data = text.data(using: .utf8),
              let any = try? JSONSerialization.jsonObject(with: data, options: [.fragmentsAllowed])
        else { return nil }
        return convert(any, depth: 0)
    }

    private static func convert(_ any: Any, depth: Int) -> JSON? {
        if depth > 64 { return nil }
        switch any {
        case is NSNull: return .null
        case let n as NSNumber:
            if CFGetTypeID(n) == CFBooleanGetTypeID() { return .bool(n.boolValue) }
            return .number(n.doubleValue)
        case let s as String: return .string(s)
        case let a as [Any]: return .array(a.compactMap { convert($0, depth: depth + 1) })
        case let d as [String: Any]:
            var o = JSONObject()
            // JSONSerialization does not keep the order; sort for stable output.
            for k in d.keys.sorted() {
                if let v = convert(d[k]!, depth: depth + 1) { o[k] = v }
            }
            return .object(o)
        default: return nil
        }
    }
}

// MARK: - Defensive readers

extension Optional where Wrapped == JSON {
    /** The object, or an empty one. */
    var obj: JSONObject {
        if case let .object(o)? = self { return o }
        return JSONObject()
    }

    /** The array, or an empty one. */
    var arr: [JSON] {
        if case let .array(a)? = self { return a }
        return []
    }

    /** A string field, cut to max characters; "" when it is not a string. */
    func str(_ max: Int = 300) -> String {
        if case let .string(s)? = self { return String(s.prefix(max)) }
        return ""
    }

    /** A string field, or nil when absent or not a string. */
    var strOrNil: String? {
        if case let .string(s)? = self { return s }
        return nil
    }

    /** A finite number; 0 otherwise. */
    var num: Double {
        if case let .number(d)? = self, d.isFinite { return d }
        return 0
    }

    /**
     * A finite number narrowed to Int without trapping: an untrusted
     * message can carry any Double (e.g. 1e300), and Swift's `Int(_:)`
     * aborts the app when the value is out of range, so these clamp to the
     * integer's range and round instead.
     */
    var int: Int { num.clampedToInt() }
    var int64: Int64 { num.clampedToInt64() }
    /** A finite number narrowed to a non-negative UInt32, never trapping. */
    var uint32: UInt32 {
        let d = num.rounded()
        if d <= 0 { return 0 }
        if d >= Double(UInt32.max) { return UInt32.max }
        return UInt32(d)
    }

    var isNumber: Bool {
        if case let .number(d)? = self { return d.isFinite }
        return false
    }

    var isTrue: Bool {
        if case .bool(true)? = self { return true }
        return false
    }

    var isFalse: Bool {
        if case .bool(false)? = self { return true }
        return false
    }
}

/** JavaScript's Math.round (halves go up), so numbers match the web. */
func jsRound(_ v: Double) -> Int { (v + 0.5).rounded(.down).clampedToInt() }

extension Double {
    /** This value rounded to the nearest Int, clamped to Int's range (no trap). */
    func clampedToInt() -> Int {
        guard isFinite else { return 0 }
        let r = rounded()
        if r >= Double(Int.max) { return Int.max }
        if r <= Double(Int.min) { return Int.min }
        return Int(r)
    }

    /** This value rounded to the nearest Int64, clamped to Int64's range (no trap). */
    func clampedToInt64() -> Int64 {
        guard isFinite else { return 0 }
        let r = rounded()
        if r >= Double(Int64.max) { return Int64.max }
        if r <= Double(Int64.min) { return Int64.min }
        return Int64(r)
    }
}

public extension Comparable {
    func clamped(_ lo: Self, _ hi: Self) -> Self { min(max(self, lo), hi) }
}
