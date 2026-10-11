// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

/**
 * The 9-digit session code shown grouped while it is typed: "915 355 636".
 * The spaces are only for the eyes (the code is the 9 digits). A field
 * that also takes an invitation link leaves a link as it is. Copied from
 * go-link's player app, which follows the same rules as its website.
 */
public enum CodeInput {
    public static let digits = 9
    public static let maxInput = 120

    /** "915355636" -> "915 355 636" (up to 9 digits, groups of 3). */
    public static func group(_ digits: String) -> String {
        var out = ""
        for (i, ch) in digits.enumerated() {
            if i > 0 && i % 3 == 0 { out.append(" ") }
            out.append(ch)
        }
        return out
    }

    /**
     * Replaces text[start, end) (character offsets) with insert and returns
     * the new text and caret. Deleting only a separator (backspace over a
     * space) also deletes the digit before it.
     */
    public static func edit(text: String, start: Int, end: Int, insert: String, allowLinks: Bool = true) -> (text: String, caret: Int) {
        let chars = Array(text)
        var s = max(0, min(start, chars.count))
        let e = max(s, min(end, chars.count))
        let raw = String(chars[0..<s]) + insert + String(chars[e...])
        if allowLinks && raw.contains(where: { !isDigit($0) && !$0.isWhitespace && $0 != "." && $0 != "-" }) {
            return (String(raw.prefix(maxInput)), min(maxInput, s + insert.count))
        }
        if insert.isEmpty && e > s && chars[s..<e].allSatisfy({ !isDigit($0) }) {
            // Backspace over a separator: take the digit before it too.
            while s > 0 && !isDigit(chars[s - 1]) { s -= 1 }
            if s > 0 { s -= 1 }
        }
        let before = chars[0..<s].filter(isDigit)
        let added = insert.filter(isDigit)
        let after = chars[e...].filter(isDigit)
        let all = String((before + added + after).prefix(digits))
        let caretDigits = min(digits, before.count + added.count)
        return (group(all), offset(afterDigits: caretDigits))
    }

    /** The caret offset right after n digits in grouped text. */
    public static func offset(afterDigits n: Int) -> Int {
        n == 0 ? 0 : n + (n - 1) / 3
    }

    static func isDigit(_ c: Character) -> Bool { ("0"..."9").contains(c) }
}
